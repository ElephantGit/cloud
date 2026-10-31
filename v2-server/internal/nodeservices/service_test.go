package nodeservices

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testModel = Model{ID: "v2-fixture", Name: "V2 fixture", ContextWindow: 128000, MaxTokens: 8192}

func gitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, data)
	}
	return strings.TrimSpace(string(data))
}

func seedRepository(t *testing.T, root string) (string, string, string) {
	t.Helper()
	seed := filepath.Join(root, "seed")
	if err := os.Mkdir(seed, 0700); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, seed, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("V2 real Node fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, seed, "add", "README.md")
	gitCommand(t, seed, "-c", "user.name=V2", "-c", "user.email=v2@example.test", "commit", "-m", "seed")
	commit := gitCommand(t, seed, "rev-parse", "HEAD")
	bare := filepath.Join(root, "repo.git")
	gitCommand(t, root, "clone", "--bare", seed, bare)
	snapshot := "refs/ora/upstreams/11111111-1111-4111-8111-111111111111/snapshots/" + commit
	gitCommand(t, bare, "update-ref", snapshot, commit)
	return bare, commit, snapshot
}

func testService(t *testing.T, provider Provider) *Service {
	t.Helper()
	return testServiceWith(t, map[string]Binding{"binding": {Model: testModel, Protocol: OpenAICompletions, Provider: provider}})
}

func testServiceWith(t *testing.T, bindings map[string]Binding) *Service {
	t.Helper()
	root := t.TempDir()
	bare, _, _ := seedRepository(t, root)
	file := filepath.Join(root, "fixture.orax")
	data := []byte("package bytes")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	s, err := Start(Options{StateDirectory: root, Repository: bare, GitBackend: filepath.Join(gitCommand(t, root, "--exec-path"), "git-http-backend"), Packages: []Package{{Name: "agent", Path: file, SHA256: hex.EncodeToString(sum[:])}}, Bindings: bindings})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func serviceClient(t *testing.T, s *Service, identity bool) *http.Client {
	t.Helper()
	ca, err := os.ReadFile(s.TLS.CACert)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(ca)
	config := &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	if identity {
		pair, err := tls.LoadX509KeyPair(s.TLS.ClientCert, s.TLS.ClientKey)
		if err != nil {
			t.Fatal(err)
		}
		config.Certificates = []tls.Certificate{pair}
	}
	transport := &http.Transport{TLSClientConfig: config}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

func request(t *testing.T, c *http.Client, method, url, token, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, data
}

func issueToken(t *testing.T, s *Service, c *http.Client, execution string) (string, string) {
	t.Helper()
	if err := s.Permit(execution, "binding"); err != nil {
		t.Fatal(err)
	}
	status, data := request(t, c, "POST", s.URL+"/internal/v1/model-grants", "", `{"bindingId":"binding","executionId":"`+execution+`"}`)
	var body struct {
		Token   string
		GrantID string
	}
	if err := json.Unmarshal(data, &body); err != nil || status != 200 || body.Token == "" || body.GrantID == "" {
		t.Fatalf("grant = %d %s %v", status, data, err)
	}
	return body.Token, body.GrantID
}

func TestServicesRequireIdentityAdmissionAndLiveGrant(t *testing.T) {
	s := testService(t, FixtureProvider{})
	auth := serviceClient(t, s, true)
	public := serviceClient(t, s, false)
	grantURL := s.URL + "/internal/v1/model-grants"
	for _, path := range []string{"/repo.git/info/refs?service=git-upload-pack", "/internal/v1/model-grants"} {
		status, _ := request(t, public, "GET", s.URL+path, "", "")
		if status != 401 {
			t.Fatalf("unauthenticated %s=%d", path, status)
		}
	}
	status, _ := request(t, auth, "POST", grantURL, "", `{"bindingId":"binding","executionId":"unadmitted"}`)
	if status != 403 {
		t.Fatalf("unadmitted=%d", status)
	}
	token, id := issueToken(t, s, auth, "execution")
	complete := s.URL + "/runtime/openai/v1/chat/completions"
	status, data := request(t, public, "POST", complete, token, `{"messages":[{"role":"user","content":"Hi"}],"stream":true}`)
	if status != 200 || !bytes.Contains(data, []byte("data: [DONE]")) {
		t.Fatalf("completion=%d %s", status, data)
	}
	status, _ = request(t, auth, "POST", grantURL+"/"+id+"/renew", "", `{"executionId":"foreign"}`)
	if status != 403 {
		t.Fatalf("foreign renew=%d", status)
	}
	status, _ = request(t, auth, "POST", grantURL+"/"+id+"/renew", "", `{"executionId":"execution"}`)
	if status != 200 {
		t.Fatalf("renew=%d", status)
	}
	status, _ = request(t, auth, "GET", s.URL+"/repo.git/info/refs?service=git-receive-pack", "", "")
	if status != 403 {
		t.Fatalf("write discovery=%d", status)
	}
	status, _ = request(t, auth, "GET", s.URL+"/repo.git/info/refs?service=git-upload-pack", "", "")
	if status != 200 {
		t.Fatalf("git discovery=%d", status)
	}
	resp, err := public.Get(s.PackageURLs["agent"])
	if err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(data) != "package bytes" {
		t.Fatalf("package=%q %v", data, err)
	}
	s.End("execution")
	status, _ = request(t, public, "POST", complete, token, `{}`)
	if status != 401 {
		t.Fatalf("revoked token=%d", status)
	}
	status, _ = request(t, auth, "POST", grantURL+"/"+id+"/renew", "", `{"executionId":"execution"}`)
	if status != 403 {
		t.Fatalf("ended renew=%d", status)
	}
}

type blockingProvider struct{ started, stopped chan struct{} }

func (p blockingProvider) Complete(w http.ResponseWriter, r *http.Request, _ Model) {
	close(p.started)
	<-r.Context().Done()
	close(p.stopped)
}

func TestEndCancelsActiveProvider(t *testing.T) {
	p := blockingProvider{make(chan struct{}), make(chan struct{})}
	s := testService(t, p)
	c := serviceClient(t, s, true)
	token, _ := issueToken(t, s, c, "execution")
	done := make(chan error, 1)
	go func() {
		req, _ := http.NewRequest("POST", s.URL+"/runtime/openai/v1/chat/completions", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := c.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		done <- err
	}()
	select {
	case <-p.started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}
	s.End("execution")
	select {
	case <-p.stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not observe revoke")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRemoteProviderForcesModelAndDoesNotLeakCredentials(t *testing.T) {
	observed := make(chan string, 1)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data map[string]any
		_ = json.NewDecoder(r.Body).Decode(&data)
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer backend-key" || data["model"] != testModel.ID {
			t.Errorf("unexpected upstream request: path=%s model=%v", r.URL.Path, data["model"])
		}
		observed <- r.Header.Get("X-Caller-Secret")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"usage\":{\"total_tokens\":7}}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	p, err := NewRemoteProvider(upstream.URL+"/v1", "backend-key", upstream.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"model":"attacker","stream":true,"messages":[]}`))
	r.Header.Set("Authorization", "Bearer agent-token")
	r.Header.Set("X-Caller-Secret", "do-not-forward")
	w := httptest.NewRecorder()
	p.Complete(w, r, testModel)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"total_tokens":7`) || <-observed != "" {
		t.Fatalf("unexpected provider response %d %s", w.Code, w.Body)
	}
	for _, url := range []string{"http://example.test/v1", "https://user:password@example.test", "https://example.test/v1?key=secret"} {
		if _, err := NewRemoteProvider(url, "key", nil); err == nil {
			t.Fatalf("accepted %s", url)
		}
	}
}

func TestRemoteProviderCancelsAndDiscardsErrorBody(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(stopped)
	}))
	defer upstream.Close()
	p, _ := NewRemoteProvider(upstream.URL, "backend-key", upstream.Client().Transport)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Complete(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{}`)).WithContext(ctx), testModel)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("provider not cancelled")
	}
	<-done
	bad := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "provider-secret-body", 401) }))
	defer bad.Close()
	p, _ = NewRemoteProvider(bad.URL, "backend-key", bad.Client().Transport)
	w := httptest.NewRecorder()
	p.Complete(w, httptest.NewRequest("POST", "/", strings.NewReader(`{}`)), testModel)
	if w.Code != 502 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("error leak: %d %s", w.Code, w.Body)
	}
}

func TestGrantReplacementExpiryIsolationAndPackageSnapshot(t *testing.T) {
	s := testService(t, FixtureProvider{})
	c := serviceClient(t, s, true)
	old, _ := issueToken(t, s, c, "execution")
	token, id := issueToken(t, s, c, "execution")
	if old == token {
		t.Fatal("grant replacement reused token")
	}
	status, _ := request(t, c, "POST", s.URL+"/runtime/openai/v1/chat/completions", old, `{}`)
	if status != 401 {
		t.Fatalf("old token=%d", status)
	}
	second := testService(t, FixtureProvider{})
	secondClient := serviceClient(t, second, true)
	issueToken(t, second, secondClient, "execution")
	status, _ = request(t, secondClient, "POST", second.URL+"/runtime/openai/v1/chat/completions", token, `{}`)
	if status != 401 {
		t.Fatalf("cross-Workspace token=%d", status)
	}
	s.mu.Lock()
	s.grants[id].expires = time.Now().Add(-time.Second)
	s.mu.Unlock()
	status, _ = request(t, c, "POST", s.URL+"/runtime/openai/v1/chat/completions", token, `{}`)
	if status != 401 {
		t.Fatalf("expired token=%d", status)
	}
	status, _ = request(t, c, "POST", s.URL+"/internal/v1/model-grants/"+id+"/renew", "", `{"executionId":"execution"}`)
	if status != 403 {
		t.Fatalf("expired renew=%d", status)
	}
	if err := os.WriteFile(s.opts.Packages[0].Path, []byte("modified source package"), 0600); err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(s.PackageURLs["agent"])
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "package bytes" {
		t.Fatalf("served changed package=%q", data)
	}
	opts := s.opts
	opts.Packages = []Package{{Name: "agent", Path: s.opts.Packages[0].Path, SHA256: strings.Repeat("0", 64)}}
	if broken, err := Start(opts); err == nil {
		broken.Close()
		t.Fatal("accepted bad package digest")
	}
}

func TestCloseJoinsProviderAndRemovesOnlyCredentials(t *testing.T) {
	p := blockingProvider{make(chan struct{}), make(chan struct{})}
	s := testService(t, p)
	c := serviceClient(t, s, true)
	token, _ := issueToken(t, s, c, "execution")
	done := make(chan struct{})
	go func() {
		defer close(done)
		req, _ := http.NewRequest("POST", s.URL+"/runtime/openai/v1/chat/completions", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+token)
		if resp, err := c.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
	select {
	case <-p.started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.stopped:
	default:
		t.Fatal("Close returned with a live provider")
	}
	<-done
	if _, err := os.Stat(s.directory); !os.IsNotExist(err) {
		t.Fatalf("credentials remain: %v", err)
	}
	if _, err := os.Stat(s.opts.Repository); err != nil {
		t.Fatalf("repository removed: %v", err)
	}
	if err := s.Permit("later", "binding"); err == nil {
		t.Fatal("admitted after Close")
	}
}
