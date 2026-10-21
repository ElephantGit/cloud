package modelgateway

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/internal/egress"
)

func TestUpstreamTimeoutConfiguration(t *testing.T) {
	for _, c := range []struct {
		name             string
		connect, headers time.Duration
		wantConnect      time.Duration
		wantHeaders      time.Duration
	}{
		{"production defaults", 0, 0, 10 * time.Second, 30 * time.Second},
		{"nonpositive retains bounds", -time.Second, -time.Second, 10 * time.Second, 30 * time.Second},
		{"injected limits", 50 * time.Millisecond, 150 * time.Millisecond, 50 * time.Millisecond, 150 * time.Millisecond},
	} {
		t.Run(c.name, func(t *testing.T) {
			client, err := NewUpstreamClientWithTimeouts(UpstreamConfig{DNS: egress.DefaultDNSConfig()}, nil,
				UpstreamTimeouts{Connect: c.connect, ResponseHeader: c.headers})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.CloseIdleConnections)
			transport := client.Transport.(*upstreamTransport)
			if transport.TLSHandshakeTimeout != c.wantConnect || transport.ResponseHeaderTimeout != c.wantHeaders {
				t.Fatalf("timeouts = %s/%s, want %s/%s", transport.TLSHandshakeTimeout, transport.ResponseHeaderTimeout, c.wantConnect, c.wantHeaders)
			}
		})
	}
}

type fixedResolver []netip.Addr

func (r fixedResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r, nil
}

func TestModelEndpointPolicyRejectsPrivateReservedAndMixedDNS(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "198.18.0.1", "::1", "::ffff:127.0.0.1", "fc00::1", "2001:db8::1", "2001:2::b", "2001:2:0:ffff:ffff:ffff:ffff:ffff", "64:ff9b::a00:1"} {
		if publicAddress(netip.MustParseAddr(value)) {
			t.Errorf("forbidden address accepted: %s", value)
		}
	}
	for _, value := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !publicAddress(netip.MustParseAddr(value)) {
			t.Errorf("public address rejected: %s", value)
		}
	}
	d := secureDialer{resolver: fixedResolver{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}}
	if _, err := d.dial(context.Background(), "tcp", "models.example.invalid:443"); err == nil {
		t.Fatal("mixed DNS answers reached a dial")
	}
}

func TestProtocolEndpointJoiningPreservesBluezonePrefixes(t *testing.T) {
	for _, test := range []struct{ base, protocol, want string }{{"https://models.example.invalid/v1/", "openai-completions", "https://models.example.invalid/v1/chat/completions"}, {"https://models.example.invalid/compatible", "anthropic-messages", "https://models.example.invalid/compatible/v1/messages"}, {"https://models.example.invalid/v1", "anthropic-messages", "https://models.example.invalid/v1/messages"}} {
		got, err := upstreamURL(test.base, test.protocol)
		if err != nil || got != test.want {
			t.Fatalf("endpoint %s expected %s", got, test.want)
		}
	}
	for _, base := range []string{"http://example.invalid", "https://user:password@example.invalid", "https://example.invalid?key=value", "https://example.invalid#fragment"} {
		if _, err := upstreamURL(base, "openai-completions"); err == nil {
			t.Fatal("unsafe model endpoint accepted")
		}
	}
}

func TestMalformedCredentialPathsCannotFallThroughToCloud(t *testing.T) {
	for _, path := range []string{"/api/v1/me/model-connections/not-a-uuid/credential", "/api/v1/me/model-connections/a/b/credential", "/api/v1/me/model-connections/a/credential/"} {
		if !IsCredentialPath(path) {
			t.Fatal("credential namespace fell through to Cloud")
		}
	}
	if IsCredentialPath("/api/v1/me/model-connections") {
		t.Fatal("metadata route treated as credential write")
	}
}
