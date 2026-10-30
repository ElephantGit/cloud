package projectrepos

import "testing"

func TestRepositoryURLPolicy(t *testing.T) {
	tests := []struct {
		url                 string
		production, fixture bool
	}{
		{"https://github.com/ora-space/demo.git", true, true},
		{"http://127.0.0.1:1234/demo.git", false, true},
		{"http://[::1]:1234/demo.git", false, true},
		{"http://example.com/demo.git", false, false},
		{"http://localhost:1234/demo.git", false, false},
		{"https://user:secret@example.com/demo.git", false, false},
		{"https://example.com/demo.git?token=secret", false, false},
		{"https://example.com/demo.git?", false, false},
		{"https://example.com/demo.git#main", false, false},
		{"ssh://git@example.com/demo.git", false, false},
		{"git@example.com:demo.git", false, false},
		{"file:///private/repository", false, false},
		{"fixture://demo", false, false},
		{"/private/repository", false, false},
		{"-c credential.helper=evil", false, false},
	}
	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			if got := (&Service{}).validURL(test.url); got != test.production {
				t.Errorf("production URL allowed=%v", got)
			}
			if got := (&Service{allowLoopbackHTTP: true}).validURL(test.url); got != test.fixture {
				t.Errorf("fixture URL allowed=%v", got)
			}
		})
	}
}
