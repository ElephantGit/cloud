package pluginmarket

import (
	"net/netip"
	"testing"
)

func TestRejectInternalMarketplaces(t *testing.T) {
	for _, s := range []string{"file:///etc", "https://key@github.com/repo", "https://example.org:22/repo", "https://example.org/repo?token=x"} {
		if _, e := ValidatePublicSource(s, "main"); e == nil {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "::ffff:192.168.0.2", "64:ff9b::a00:1", "100.64.0.2"} {
		if publicIP(netip.MustParseAddr(s)) {
			t.Fatal(s)
		}
	}
	if !publicIP(netip.MustParseAddr("140.82.112.3")) {
		t.Fatal("public Git host rejected")
	}
}

func TestNamespaceMatchesDesktop(t *testing.T) {
	u, e := ValidatePublicSource("https://GitHub.com/Acme/Awesome-Plugins.git/", "main")
	if e != nil || Namespace(u) != "awesome-plugins.2aa64f48" {
		t.Fatal(u, e)
	}
}

func TestSelectDialAddress(t *testing.T) {
	for _, tc := range []struct {
		name string
		host string
		ips  []string
		want string
		err  bool
	}{
		{
			name: "public DNS answer",
			host: "github.com",
			ips:  []string{"140.82.112.3"},
			want: "140.82.112.3",
		},
		{
			name: "synthetic DNS answer",
			host: "github.com",
			ips:  []string{"198.18.0.27", "2001:2::1b"},
			want: "198.18.0.27",
		},
		{
			name: "synthetic DNS answer alone",
			host: "github.com",
			ips:  []string{"198.18.0.130"},
			want: "198.18.0.130",
		},
		{
			name: "synthetic DNS mixed with private answer",
			host: "github.com",
			ips:  []string{"198.18.0.27", "10.0.0.1"},
			err:  true,
		},
		{
			name: "literal synthetic address",
			host: "198.18.0.27",
			ips:  []string{"198.18.0.27"},
			err:  true,
		},
		{
			name: "literal private address",
			host: "10.0.0.1",
			ips:  []string{"10.0.0.1"},
			err:  true,
		},
		{
			name: "DNS private address",
			host: "example.org",
			ips:  []string{"10.0.0.1"},
			err:  true,
		},
		{
			name: "empty DNS answer",
			host: "example.org",
			err:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ips := make([]netip.Addr, 0, len(tc.ips))
			for _, raw := range tc.ips {
				ips = append(ips, netip.MustParseAddr(raw))
			}
			got, e := selectDialAddress(tc.host, ips)
			if tc.err {
				if e == nil {
					t.Fatalf("selectDialAddress(%q, %v) = %s, want error", tc.host, ips, got)
				}
				return
			}
			if e != nil || got.String() != tc.want {
				t.Fatalf("selectDialAddress(%q, %v) = %s, %v; want %s", tc.host, ips, got, e, tc.want)
			}
		})
	}
}
