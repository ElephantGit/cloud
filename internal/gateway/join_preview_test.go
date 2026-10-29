package gateway

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wanglongan587/cloud/internal/core"
)

func TestAnonymousJoinPreviewThroughRealGateway(t *testing.T) {
	server, _, _, observed := queryTestGateway(t)
	for _, tc := range []struct {
		method, path, origin string
		status               int
	}{
		{"POST", core.JoinLinkPreviewPath, "http://localhost", 200},
		{"POST", core.JoinLinkPreviewPath, "https://other.example", 403},
		{"POST", "/api/v1/join/invitations/redeem", "http://localhost", 401},
		{"GET", core.JoinLinkPreviewPath, "http://localhost", 401},
	} {
		body := `{"kind":"invite","token":"` + strings.Repeat("A", 43) + `"}`
		request, err := http.NewRequest(tc.method, server.URL+tc.path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", tc.origin)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Ora-User-Token", "forged-browser-credential")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != tc.status {
			t.Fatalf("%s %s: %d %s err=%v", tc.method, tc.path, response.StatusCode, payload, err)
		}
		if tc.status == 200 {
			if !strings.Contains(string(payload), `"state":"unavailable"`) || response.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("minimal uncached preview: %s", payload)
			}
			<-observed
		}
	}
}
