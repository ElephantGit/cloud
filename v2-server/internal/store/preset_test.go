package store_test

import (
	"strings"
	"testing"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
)

func TestParsePresetValidatesAccounts(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string // empty means the preset is valid
	}{
		{name: "minimum", raw: `{"users":[{"email":"Owner@Demo.test","displayName":"","role":"owner"}]}`},
		{name: "retired presets", raw: `{"users":[{"email":"Owner@Demo.test","role":"owner"}],"retired":[{"email":"Old@Demo.test"}]}`},
		{name: "empty", raw: `{"users":[]}`, want: "no users"},
		{name: "missing users", raw: `{}`, want: "no users"},
		{name: "no owner", raw: `{"users":[{"email":"a@demo.test","displayName":"A","role":"member"}]}`, want: "no owner"},
		{name: "unknown role", raw: `{"users":[{"email":"a@demo.test","role":"admin"}]}`, want: "is not owner or member"},
		{name: "bad email", raw: `{"users":[{"email":"a@","role":"owner"}]}`, want: "invalid email"},
		{name: "duplicate", raw: `{"users":[{"email":"a@demo.test","role":"owner"},{"email":"A@demo.test","role":"member"}]}`, want: "duplicate email"},
		{name: "retired overlaps a user", raw: `{"users":[{"email":"a@demo.test","role":"owner"}],"retired":[{"email":"A@demo.test"}]}`, want: "duplicate email"},
		{name: "unknown field", raw: `{"users":[{"email":"a@demo.test","role":"owner","password":"x"}]}`, want: "unknown field"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, e := store.ParsePreset([]byte(c.raw))
			if c.want != "" {
				if e == nil || !strings.Contains(e.Error(), c.want) {
					t.Fatalf("error = %v, want %q", e, c.want)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			// Emails are normalized and a missing display name falls back to the local part.
			if p.Users[0].Email != "owner@demo.test" || p.Users[0].DisplayName != "owner" {
				t.Fatalf("preset user = %+v", p.Users[0])
			}
		})
	}
}
