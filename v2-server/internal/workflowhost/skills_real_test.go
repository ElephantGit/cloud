//go:build linux

package workflowhost

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/agentruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/executionruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
	"github.com/wanglongan587/cloud/v2-server/internal/pluginmarket"
	"github.com/wanglongan587/cloud/v2-server/internal/projectrepos"
	"github.com/wanglongan587/cloud/v2-server/internal/server"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/store/storetest"
)

// The model supplies a deterministic tool request; Node, OpenCode, PostgreSQL, the published
// workflow, official Skill package and shell file reads all cross their production boundaries.
type workflowSkillFixture struct{}

func (workflowSkillFixture) Complete(w http.ResponseWriter, r *http.Request, m nodeservices.Model) {
	buffer := httptest.NewRecorder()
	nodeservices.FixtureProvider{}.Complete(buffer, r, m)
	for k, vs := range buffer.Header() {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(buffer.Code)
	command := "(if [ -f .opencode/skills/ask-matt/SKILL.md ]; then cp .opencode/skills/ask-matt/SKILL.md read-plugin.md; else echo absent > next-plugin.txt; fi; if [ -f .opencode/skills/workflow-review/SKILL.md ]; then cp .opencode/skills/workflow-review/SKILL.md read-personal.md; else echo absent > next-personal.txt; fi; cat README.md"
	_, _ = w.Write(bytes.ReplaceAll(buffer.Body.Bytes(), []byte("(cat README.md"), []byte(command)))
}

func TestRealWorkflowSelectedSkills(t *testing.T) {
	assets, archive := os.Getenv("V2_NODE_ASSETS_DIR"), os.Getenv("V2_TEST_SKILL_ARCHIVE")
	if assets == "" || archive == "" {
		t.Skip("real Node assets and official Skill archive required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(home, "vw-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	db := storetest.Open(t)
	user, tenant := storetest.Login(t, db, "workflow-skills")
	scope := execution.Scope{TenantID: tenant, UserID: user}
	repositories, err := projectrepos.New(db, projectrepos.Options{Root: filepath.Join(root, "r")})
	if err != nil {
		t.Fatal(err)
	}
	project, err := repositories.SeedDemo(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	models, err := executionruntime.ModelsFromEnvironment(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	model := models["fixture"]
	model.Binding.Provider = workflowSkillFixture{}
	models["fixture"] = model
	gitPath, err := exec.CommandContext(ctx, "git", "--exec-path").Output()
	if err != nil {
		t.Fatal(err)
	}
	driver, err := agentruntime.New(agentruntime.Options{
		Root: filepath.Join(root, "n"), AssetsDirectory: assets,
		GitBackend: filepath.Join(strings.TrimSpace(string(gitPath)), "git-http-backend"),
		Bindings:   map[string]nodeservices.Binding{"fixture": model.Binding},
		ResolveProject: func(c context.Context, s execution.Scope, id string) (agentruntime.Project, error) {
			p, e := db.GetProject(c, s, id)
			return agentruntime.Project{MirrorPath: p.MirrorPath, Commit: p.Commit, SnapshotRef: p.SnapshotRef}, e
		},
		RecordWorkspace: func(c context.Context, id string, at agentruntime.Checkout) error {
			return db.RecordWorkspace(c, id, store.WorkspaceLocation{Path: at.Path, BaseCommit: at.BaseCommit})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := driver.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	service, err := executionruntime.New(ctx, executionruntime.Config{Store: db, Models: models, Agent: driver})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	// As in v2, Skill installation waits for the project's first session to bring Node online.
	ref, err := service.Start(ctx, scope, execution.StartRequest{Caller: execution.Caller{Kind: execution.CallerChat, ID: "warm", AttemptID: "warm"}, Target: execution.Target{ProjectID: project.ID}, ExecutorID: "opencode:fixture", Prompt: execution.TextPrompt("Read README.")})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := service.Subscribe(ctx, scope, ref.SessionID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for {
		event, err := stream.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if event.Kind == execution.EventTurnEnded {
			if event.Result == nil || event.Result.Status != execution.StatusCompleted {
				t.Fatal(event.Result)
			}
			break
		}
	}
	if err := service.End(ctx, scope, ref.SessionID); err != nil {
		t.Fatal(err)
	}
	const packageID = "official/mattpocock.skills"
	const digest = "c3de8a2c6863a1864901fc55e7eb42e22fe0829fb958249de59c5d4e715def64"
	entry := pluginmarket.Entry{Namespace: "official", Identifier: "mattpocock.skills", Kind: "skill", Version: "1.3.1", MarketplaceVisible: true, URL: "https://github.com/ora-space/mattpocock-skill/releases/download/v1.3.1/mattpocock.skills-v1.3.1.orax", SHA256: digest}
	if err := db.ReplacePluginCatalog(ctx, []pluginmarket.Entry{entry}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InstallSpacePlugin(ctx, tenant, project.SpaceID, user, "plugins", "install", store.PluginInstallInput{Identifier: packageID}); err != nil {
		t.Fatal(err)
	}
	jobs, err := db.SkillJobs(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatal(jobs, err)
	}
	if ok, err := db.StartSkillJob(ctx, jobs[0]); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := driver.ApplySkill(ctx, project.ID, packageID, "1.3.1", archive, digest, false); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishSkillJob(ctx, jobs[0], store.SkillJobSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateSkill(ctx, tenant, user, store.SkillInput{Name: "workflow-review", Description: "Review", Content: "SELECTED_WORKFLOW_INSTRUCTIONS"}); err != nil {
		t.Fatal(err)
	}
	host, err := NewHost(Config{Store: db, Execution: service, Catalog: service})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	f := &fixture{s: db, host: host, scope: server.WorkflowRunScope{TenantID: tenant, UserID: user}, project: project.ID}
	graph := strings.Replace(twoAgents, `"roleId":"reviewer"`, `"skills":[{"skillId":"official/mattpocock.skills","enabled":true},{"skillId":"workflow-review","enabled":true}]`, 1)
	rid := f.run(t, graph, str("Read selected Skills and retain evidence."))
	if _, err := host.StartRun(ctx, f.scope, rid); err != nil {
		t.Fatal(err)
	}
	events, unsubscribe := host.Hub().Subscribe(rid)
	defer unsubscribe()
	for {
		run, err := db.WorkflowRun(ctx, tenant, rid)
		if err != nil {
			t.Fatal(err)
		}
		if terminal(run, nil) {
			if run.Status != store.WorkflowStatusSucceeded {
				t.Fatalf("workflow failed: %+v", run)
			}
			break
		}
		select {
		case <-events:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	nodes, err := db.WorkflowEngineNodeRuns(ctx, rid)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		n := node(nodes, id)
		if n.SessionID == nil {
			t.Fatalf("node %s has no session", id)
		}
		s, err := service.ReadSession(ctx, scope, *n.SessionID)
		if err != nil || s.Skills == nil {
			t.Fatal(s, err)
		}
		if id == "a" && (len(s.Skills.Plugins) != 1 || len(s.Skills.Documents) != 1 || s.Skills.Documents[0].Content != "SELECTED_WORKFLOW_INSTRUCTIONS") {
			t.Fatalf("lost selected skills: %+v", s.Skills)
		}
		if id == "b" && (len(s.Skills.Plugins) != 0 || len(s.Skills.Documents) != 0) {
			t.Fatal("inherited previous node's skills")
		}
		turns, err := service.ReadTurns(ctx, scope, *n.SessionID, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(turns)
		if id == "a" && !bytes.Contains(raw, []byte("required_skills")) {
			t.Fatal("missing required Skill prompt")
		}
	}
	at, err := db.WorkflowRunWorkspace(ctx, tenant, rid)
	if err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{"read-plugin.md": "ask-matt", "read-personal.md": "SELECTED_WORKFLOW_INSTRUCTIONS", "next-plugin.txt": "absent", "next-personal.txt": "absent"} {
		data, err := os.ReadFile(filepath.Join(at.Path, file))
		if err != nil || !strings.Contains(string(data), want) {
			t.Fatalf("%s: %s %v", file, data, err)
		}
	}
}
