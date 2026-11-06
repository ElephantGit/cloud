package store

import "testing"

func TestDemoSeedRunUsesBothPersistedIdentities(t *testing.T) {
	for _, test := range []struct {
		name, start, session string
		want                 bool
	}{
		{"nonexecuting example", DemoSeedSessionPrefix + "project", DemoSeedSessionPrefix + "project", true},
		{"normal fixture execution", "fixture-check", "execution-uuid", false},
		{"chosen seed-like key", DemoSeedSessionPrefix + "project", "execution-uuid", false},
		{"mismatching identities", DemoSeedSessionPrefix + "one", DemoSeedSessionPrefix + "two", false},
		{"not attached", DemoSeedSessionPrefix + "project", "", false},
		{"missing project", DemoSeedSessionPrefix, DemoSeedSessionPrefix, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isDemoSeedRun(test.start, test.session); got != test.want {
				t.Fatalf("isDemoSeedRun(%q, %q) = %v, want %v", test.start, test.session, got, test.want)
			}
		})
	}
}
