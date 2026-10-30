//go:build linux

package executionruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wanglongan587/cloud/v2-server/internal/agentruntime"
	"github.com/wanglongan587/cloud/v2-server/internal/execution"
	"github.com/wanglongan587/cloud/v2-server/internal/nodeservices"
	"github.com/wanglongan587/cloud/v2-server/internal/projectrepos"
	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

const configuredMarkerName = "ora-v2-configured-provider.txt"
const configuredMarker = "v2 configured provider tool executed\n"

// TestConfiguredProviderServicePostgresRestart deliberately makes external model requests only
// after explicit opt-in. It does not retry a request or turn; OpenCode retains its own strategy.
func TestConfiguredProviderServicePostgresRestart(t *testing.T) {
	if os.Getenv("V2_REAL_PROVIDER_TEST") != "1" {
		t.Skip("external provider NOT verified; set V2_REAL_PROVIDER_TEST=1 with all required configuration to opt in")
	}
	for _, name := range []string{"V2_MODEL_BASE_URL", "V2_MODEL_API_KEY", "V2_MODEL_NAME", "TEST_DATABASE_URL", "V2_NODE_ASSETS_DIR"} {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			t.Fatalf("explicit external provider acceptance requires %s", name)
		}
	}
	models, err := ModelsFromEnvironment(os.Getenv)
	configuredOK(t, "configure external model", err)
	if _, ok := models["configured"]; !ok {
		t.Fatal("configured model is unavailable; fixture cannot satisfy external acceptance")
	}
	assets := os.Getenv("V2_NODE_ASSETS_DIR")
	if !filepath.IsAbs(assets) {
		t.Fatal("V2_NODE_ASSETS_DIR must be an absolute native Linux path")
	}
	_, err = os.Stat(filepath.Join(assets, "assets.json"))
	configuredOK(t, "locate prepared Node assets", err)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	gitPath, err := exec.CommandContext(ctx, "git", "--exec-path").Output()
	configuredOK(t, "locate Git backend", err)
	gitBackend := filepath.Join(strings.TrimSpace(string(gitPath)), "git-http-backend")
	_, err = os.Stat(gitBackend)
	configuredOK(t, "locate Git HTTP executable", err)
	f := configuredDatabase(t, ctx, models)
	driverRoot := filepath.Join(f.root, "a")
	newService := func() {
		t.Helper()
		f.driver, err = agentruntime.New(agentruntime.Options{
			Root: driverRoot, AssetsDirectory: assets, GitBackend: gitBackend,
			Bindings: map[string]nodeservices.Binding{"configured": models["configured"].Binding},
			ResolveProject: func(ctx context.Context, scope execution.Scope, id string) (agentruntime.Project, error) {
				project, err := f.db.GetProject(ctx, scope, id)
				return agentruntime.Project{MirrorPath: project.MirrorPath, Commit: project.Commit, SnapshotRef: project.SnapshotRef}, err
			},
		})
		configuredOK(t, "initialize real Agent driver (requires a short native Linux home path)", err)
		f.service, err = New(ctx, Config{Store: f.db, Models: models, Agent: f.driver})
		configuredOK(t, "initialize public execution Service", err)
	}
	newService()
	repositories, err := projectrepos.New(f.db, projectrepos.Options{Root: filepath.Join(f.root, "r")})
	configuredOK(t, "initialize isolated project repository", err)
	project, err := repositories.SeedDemo(ctx, f.scope)
	configuredOK(t, "seed isolated real Git project", err)
	reopen := func() {
		t.Helper()
		configuredOK(t, "close execution Service and its Node processes", f.service.Close(ctx))
		f.service, f.driver = nil, nil
		f.db.Close()
		f.db = nil
		f.db, err = store.OpenConfig(ctx, f.config.Copy())
		configuredOK(t, "reopen isolated PostgreSQL pool", err)
		newService()
	}

	modelCaller := execution.Caller{Kind: execution.CallerChat, ID: "external-model", AttemptID: "first"}
	modelRef, err := f.service.StartModel(ctx, f.scope, execution.ModelStartRequest{
		Caller: modelCaller, ModelID: "configured",
		Prompt: execution.TextPrompt("Reply with one short plain-text sentence confirming you can answer. Do not use tools."),
	})
	configuredOK(t, "StartModel admission", err)
	modelResult, _ := observeTurn(t, &f.runtimeFixture, modelRef, 0)
	configuredComplete(t, "model", modelResult)
	configuredOK(t, "End model session", f.service.End(ctx, f.scope, modelRef.SessionID))
	reopen()
	configuredReadBack(t, f, modelCaller, modelRef, modelResult)

	agentCaller := execution.Caller{Kind: execution.CallerChat, ID: "external-agent", AttemptID: "first"}
	agentRef, err := f.service.Start(ctx, f.scope, execution.StartRequest{
		Caller: agentCaller, Target: execution.Target{ProjectID: project.ID}, ExecutorID: "opencode:configured",
		Prompt: execution.TextPrompt("Use the shell tool in this repository to read README.md, then run exactly:\n```sh\nprintf '%s\\n' 'v2 configured provider tool executed' > " + configuredMarkerName + "\n```\nDo not modify any other files. After the tool succeeds, reply with one short plain-text confirmation sentence."),
	})
	configuredOK(t, "Start real Agent admission", err)
	agentResult, _ := observeTurn(t, &f.runtimeFixture, agentRef, 0)
	configuredComplete(t, "Agent", agentResult)
	marker := configuredToolMarker(t, driverRoot)
	configuredOK(t, "End Agent session", f.service.End(ctx, f.scope, agentRef.SessionID))
	reopen()
	configuredReadBack(t, f, modelCaller, modelRef, modelResult)
	configuredReadBack(t, f, agentCaller, agentRef, agentResult)
	if configuredToolMarker(t, driverRoot) != marker {
		t.Fatal("End/restart did not retain the same tool-written marker")
	}
	// Never print answers, provider/DB URLs, credentials or raw Node diagnostics, even on failure.
	t.Log("external model and Agent completed; real tool marker and PostgreSQL results survived End and pool/Service restart")
}

type configuredProviderFixture struct {
	runtimeFixture
	admin  *pgx.Conn
	schema string
	root   string
	driver *agentruntime.Driver
}

// Keep setup errors opaque: database/client errors can embed connection strings. The existing
// fixture's public-port observer is safe to reuse, but its raw setup diagnostics are not.
func configuredDatabase(t *testing.T, ctx context.Context, models map[string]ModelChoice) *configuredProviderFixture {
	t.Helper()
	home, err := os.UserHomeDir()
	configuredOK(t, "locate native Linux home", err)
	root, err := os.MkdirTemp(home, "vp-")
	configuredOK(t, "create private acceptance directory", err)
	f := &configuredProviderFixture{runtimeFixture: runtimeFixture{ctx: ctx, models: models}, root: root}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 90*time.Second)
		defer stop()
		stopped := true
		if f.service != nil && f.service.Close(cleanup) != nil {
			t.Error("acceptance Service shutdown failed; retaining isolated evidence")
			stopped = false
		}
		if f.driver != nil && f.driver.Close(cleanup) != nil {
			t.Error("acceptance Node shutdown unconfirmed; retaining isolated evidence")
			stopped = false
		}
		if !stopped {
			return // Never delete a live runtime's files or close a worker's database underneath it.
		}
		if f.db != nil {
			f.db.Close()
		}
		if f.admin != nil {
			if f.schema != "" {
				if _, err := f.admin.Exec(cleanup, "DROP SCHEMA "+pgx.Identifier{f.schema}.Sanitize()+" CASCADE"); err != nil {
					t.Error("remove isolated acceptance schema failed")
				}
			}
			if f.admin.Close(cleanup) != nil {
				t.Error("close acceptance database admin connection failed")
			}
		}
		if agentruntime.Cleanup(cleanup, filepath.Join(root, "a")) != nil {
			t.Error("owned Node cleanup failed; retaining isolated directory")
			return
		}
		// root was created by this test, and registered Node ownership was confirmed above.
		if os.RemoveAll(root) != nil {
			t.Error("remove private acceptance directory failed")
		}
	})
	f.admin, err = pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
	configuredOK(t, "connect PostgreSQL for isolated schema", err)
	var id [8]byte
	_, err = rand.Read(id[:])
	configuredOK(t, "allocate isolated schema identity", err)
	schema := "configured_provider_" + hex.EncodeToString(id[:])
	_, err = f.admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	configuredOK(t, "create isolated PostgreSQL schema", err)
	f.schema = schema // Cleanup owns only a schema this test successfully created.
	f.config, err = pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	configuredOK(t, "parse test database configuration", err)
	f.config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	f.db, err = store.OpenConfig(ctx, f.config.Copy())
	configuredOK(t, "open isolated PostgreSQL store", err)
	token, err := f.db.Login(ctx, store.Identity{Source: "test", Subject: "configured-provider", DisplayName: "Acceptance"}, time.Hour)
	configuredOK(t, "create isolated acceptance member", err)
	user, err := f.db.SessionUser(ctx, token)
	configuredOK(t, "read isolated acceptance member", err)
	tenants, err := f.db.Tenants(ctx, user.ID)
	configuredOK(t, "read isolated acceptance tenant", err)
	if len(tenants) != 1 {
		t.Fatal("acceptance member must belong to exactly one isolated tenant")
	}
	f.scope = execution.Scope{TenantID: tenants[0].ID, UserID: user.ID}
	return f
}

func configuredOK(t *testing.T, stage string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s failed (raw error omitted to protect configuration)", stage)
	}
}

func configuredComplete(t *testing.T, mode string, result execution.Result) {
	t.Helper()
	if result.Status != execution.StatusCompleted || !result.AnswerComplete || strings.TrimSpace(result.Answer) == "" {
		t.Fatalf("%s did not produce a completed, complete, nonempty text answer", mode)
	}
}

func configuredReadBack(t *testing.T, f *configuredProviderFixture, caller execution.Caller, ref execution.Ref, want execution.Result) {
	t.Helper()
	turn, err := f.service.ReadAttempt(f.ctx, f.scope, caller)
	configuredOK(t, "read persisted attempt after restart", err)
	if turn.Ref != ref || turn.Result == nil || !reflect.DeepEqual(*turn.Result, want) {
		t.Fatal("reopened PostgreSQL attempt/result differs from its complete terminal event")
	}
	session, err := f.service.ReadSession(f.ctx, f.scope, ref.SessionID)
	configuredOK(t, "read ended session after restart", err)
	if session.EndedAt == nil {
		t.Fatal("End was not retained across pool/Service restart")
	}
}

func configuredToolMarker(t *testing.T, root string) string {
	t.Helper()
	var markers []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.Name() != configuredMarkerName {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() != int64(len(configuredMarker)) {
			t.Fatal("tool marker is not a regular file of the expected size")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != configuredMarker {
			t.Fatal("tool-written marker content does not match the requested fixed value")
		}
		markers = append(markers, path)
		return nil
	})
	configuredOK(t, "inspect actual Agent checkout marker", err)
	if len(markers) != 1 {
		t.Fatalf("real Agent must write exactly one marker; found %d", len(markers))
	}
	return markers[0]
}
