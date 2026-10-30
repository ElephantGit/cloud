package nodeservices

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteProviderLogsStatusWithoutSecrets(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	for _, status := range []int{http.StatusFound, http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			logs.Reset()
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://example.test/?key=redirect-secret")
				http.Error(w, "upstream-secret-body", status)
			}))
			defer upstream.Close()
			p, err := NewRemoteProvider(upstream.URL+"/url-secret", "backend-secret-key", upstream.Client().Transport)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("POST", "/", strings.NewReader(`{"messages":[{"role":"user","content":"prompt-secret"}]}`))
			r.Header.Set("Authorization", "Bearer agent-secret-token")
			w := httptest.NewRecorder()
			p.Complete(w, r, testModel)
			if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "provider_rejected") {
				t.Fatalf("response = %d %s", w.Code, w.Body)
			}
			var entry struct {
				Status int    `json:"status"`
				Level  string `json:"level"`
			}
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatalf("expected one structured diagnostic: %v", err)
			}
			if entry.Status != status || entry.Level != "WARN" {
				t.Fatalf("diagnostic = %+v, want upstream status %d", entry, status)
			}
			for _, secret := range []string{upstream.URL, "url-secret", "backend-secret-key", "upstream-secret-body", "redirect-secret", "agent-secret-token", "prompt-secret"} {
				if strings.Contains(logs.String(), secret) || strings.Contains(w.Body.String(), secret) {
					t.Errorf("diagnostic or response exposed %q", secret)
				}
			}
		})
	}
}
