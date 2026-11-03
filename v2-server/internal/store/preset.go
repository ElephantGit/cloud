package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// EmailSource is the users.source of every browser account: the email alone is the identity.
const EmailSource = "email"

// The only workspace roles v2 knows. Cloud's admin vocabulary is not part of the demo.
const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

// PresetUser is one account preset by the startup configuration file.
type PresetUser struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

// RetiredUser is one account from an older preset that must no longer appear as a member.
// The cleanup runs once so a later invitation of the same address is not removed.
type RetiredUser struct {
	Email string `json:"email"`
}

// Preset is the startup configuration: the accounts that may sign in before any invitation.
type Preset struct {
	Users   []PresetUser  `json:"users"`
	Retired []RetiredUser `json:"retired,omitempty"`
}

// LoadPreset reads and validates the startup accounts file. Browsers may only sign in accounts
// listed here or invited later, so an unreadable or empty file is an error rather than an empty
// team nobody can enter.
func LoadPreset(path string) (Preset, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return Preset{}, fmt.Errorf("read preset %s: %w", path, e)
	}
	p, e := ParsePreset(raw)
	if e != nil {
		return Preset{}, fmt.Errorf("preset %s: %w", path, e)
	}
	return p, nil
}

// ParsePreset decodes and validates a preset in memory.
func ParsePreset(raw []byte) (Preset, error) {
	var p Preset
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&p); e != nil {
		return Preset{}, fmt.Errorf("decode: %w", e)
	}
	if e := p.Validate(); e != nil {
		return Preset{}, e
	}
	return p, nil
}

// Validate normalizes every account in place and rejects a preset the demo cannot run from:
// an invalid or duplicate email, an unknown role, or no owner at all.
func (p *Preset) Validate() error {
	if len(p.Users) == 0 {
		return errors.New("no users")
	}
	owners, seen := 0, make(map[string]bool, len(p.Users))
	for i := range p.Users {
		u := &p.Users[i]
		email, e := NormalizeEmail(u.Email)
		if e != nil {
			return fmt.Errorf("user %d: %w", i+1, e)
		}
		if seen[email] {
			return fmt.Errorf("user %d: duplicate email %s", i+1, email)
		}
		seen[email] = true
		u.Email = email
		u.DisplayName = strings.TrimSpace(u.DisplayName)
		if u.DisplayName == "" {
			u.DisplayName = emailLocalPart(email)
		}
		switch u.Role {
		case RoleOwner:
			owners++
		case RoleMember:
		default:
			return fmt.Errorf("user %d: role %q is not %s or %s", i+1, u.Role, RoleOwner, RoleMember)
		}
	}
	for i := range p.Retired {
		email, e := NormalizeEmail(p.Retired[i].Email)
		if e != nil {
			return fmt.Errorf("retired user %d: %w", i+1, e)
		}
		if seen[email] {
			return fmt.Errorf("retired user %d: duplicate email %s", i+1, email)
		}
		seen[email] = true
		p.Retired[i].Email = email
	}
	if owners == 0 {
		return errors.New("no owner")
	}
	return nil
}

// NormalizeEmail lowercases and trims an address and rejects anything that is not one. The whole
// identity is this string, so it is validated at every boundary that turns input into an account.
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 || strings.Count(email, "@") != 1 || strings.ContainsAny(email, " \t\r\n") {
		return "", fmt.Errorf("invalid email %q", raw)
	}
	return email, nil
}

// emailLocalPart is the default display name of an invited address.
func emailLocalPart(email string) string {
	if at := strings.IndexByte(email, '@'); at > 0 {
		return email[:at]
	}
	return email
}
