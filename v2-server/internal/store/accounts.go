package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// SeedUsers applies the startup preset idempotently: each listed account is created, or has its
// display name and role refreshed, inside the demo team. Accounts missing from the file are left
// alone, so an owner's invitation survives a restart with an older file.
func (s *Store) SeedUsers(ctx context.Context, p Preset) error {
	for _, u := range p.Users {
		if _, e := s.UpsertUser(ctx, u); e != nil {
			return e
		}
	}
	return nil
}

// RetireUsers soft-deletes accounts retired from the startup preset. The cleanup marker makes it
// one-time: if the same address is invited later, a later restart must not remove that invitation.
func (s *Store) RetireUsers(ctx context.Context, retired []RetiredUser) error {
	if len(retired) == 0 {
		return nil
	}
	const cleanup = "retired-preset-users-v1"
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var applied bool
		if e := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM v2_data_cleanups WHERE name = $1)`, cleanup).Scan(&applied); e != nil {
			return fmt.Errorf("read preset cleanup marker: %w", e)
		}
		if applied {
			return nil
		}
		for _, u := range retired {
			email, e := NormalizeEmail(u.Email)
			if e != nil {
				return e
			}
			var userID string
			e = tx.QueryRow(ctx, `
				UPDATE users
				SET status = 'disabled', deleted_at = COALESCE(deleted_at, now()), version = version + 1
				WHERE source = $1 AND subject = $2 AND deleted_at IS NULL
				RETURNING id::text`, EmailSource, email).Scan(&userID)
			if errors.Is(e, pgx.ErrNoRows) {
				continue
			}
			if e != nil {
				return fmt.Errorf("retire preset user %s: %w", email, e)
			}
			if _, e := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); e != nil {
				return fmt.Errorf("delete retired preset sessions: %w", e)
			}
		}
		if _, e := tx.Exec(ctx, `INSERT INTO v2_data_cleanups (name) VALUES ($1)`, cleanup); e != nil {
			return fmt.Errorf("record preset cleanup: %w", e)
		}
		return nil
	})
}

// RetireLegacyDemoUser hides the fixed account created by the removed development login. Its
// memberships and authored rows stay in place so historical references remain intact.
func (s *Store) RetireLegacyDemoUser(ctx context.Context) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var userID string
		e := tx.QueryRow(ctx, `
			UPDATE users
			SET status = 'disabled', deleted_at = COALESCE(deleted_at, now()), version = version + 1
			WHERE source = 'dev' AND subject = 'demo' AND deleted_at IS NULL
			RETURNING id::text`).Scan(&userID)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return fmt.Errorf("retire legacy demo user: %w", e)
		}
		if _, e := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); e != nil {
			return fmt.Errorf("delete legacy demo sessions: %w", e)
		}
		return nil
	})
}

// EndDeletedSquadMembers ends the live squad memberships of soft-deleted accounts. Retiring an
// account keeps its rows for history, but a member who can no longer sign in must not count as a
// squad's member or maintainer; the ended row stays in the table, unlisted. It runs on every
// startup after the retirements, so databases retired by an older build converge too, and it is a
// no-op once nothing is left.
func (s *Store) EndDeletedSquadMembers(ctx context.Context) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `
			UPDATE v2_squad_members m SET ended_at = now()
			FROM users u
			WHERE u.id = m.user_id AND u.deleted_at IS NOT NULL AND m.ended_at IS NULL
			RETURNING m.squad_id::text`)
		if e != nil {
			return fmt.Errorf("end deleted squad members: %w", e)
		}
		squads, e := pgx.CollectRows(rows, pgx.RowTo[string])
		if e != nil {
			return fmt.Errorf("end deleted squad members: %w", e)
		}
		// A maintainer's edit read before this cleanup must see the version move, like a manual removal.
		if _, e := tx.Exec(ctx, `
			UPDATE v2_squads SET version = version + 1, updated_at = now()
			WHERE id = ANY($1::uuid[])`, squads); e != nil {
			return fmt.Errorf("bump squads of deleted members: %w", e)
		}
		return nil
	})
}

// UpsertUser creates or updates one account and its demo-team membership, returning the user id.
// It is the write behind both the startup preset and the test fixtures; browsers never reach it.
// A soft-deleted account is never resurrected: listing it again refreshes nothing but its name, and
// it gets no starter catalog, because login and invitation both refuse a deleted address.
func (s *Store) UpsertUser(ctx context.Context, in PresetUser) (string, error) {
	email, e := NormalizeEmail(in.Email)
	if e != nil {
		return "", e
	}
	if in.Role != RoleOwner && in.Role != RoleMember {
		return "", fmt.Errorf("role %q is not %s or %s", in.Role, RoleOwner, RoleMember)
	}
	var userID string
	var live bool
	e = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var tenantID string
		if e := tx.QueryRow(ctx, `
			INSERT INTO tenants (name) VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = excluded.name
			RETURNING id`, DemoTenant).Scan(&tenantID); e != nil {
			return fmt.Errorf("upsert tenant: %w", e)
		}
		if e := tx.QueryRow(ctx, `
			INSERT INTO users (source, subject, display_name) VALUES ($1, $2, $3)
			ON CONFLICT (source, subject) DO UPDATE SET display_name = excluded.display_name
			RETURNING id, deleted_at IS NULL`, EmailSource, email, in.DisplayName).Scan(&userID, &live); e != nil {
			return fmt.Errorf("upsert user: %w", e)
		}
		if !live {
			return nil
		}
		if _, e := tx.Exec(ctx, `
			INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $2, $3)
			ON CONFLICT (tenant_id, user_id) DO UPDATE SET role = excluded.role`,
			tenantID, userID, in.Role); e != nil {
			return fmt.Errorf("join tenant: %w", e)
		}
		if e := ensureDemoSpace(ctx, tx, tenantID, userID); e != nil {
			return e
		}
		return seedStarterSkills(ctx, tx, tenantID, userID)
	})
	if e != nil {
		return "", e
	}
	return userID, nil
}

// UserByEmail resolves an active account of the demo team by email. It never creates one: an
// address nobody presetted or invited is exactly what login must reject.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	email, e := NormalizeEmail(email)
	if e != nil {
		return User{}, e
	}
	var u User
	e = s.pool.QueryRow(ctx, `
		SELECT u.id, u.display_name, u.status, u.version, u.created_at, u.deleted_at
		FROM users u
		JOIN memberships m ON m.user_id = u.id
		JOIN tenants t ON t.id = m.tenant_id
		WHERE u.source = $1 AND u.subject = $2 AND t.name = $3
			AND u.status = 'active' AND u.deleted_at IS NULL AND t.status = 'active'`,
		EmailSource, email, DemoTenant).
		Scan(&u.ID, &u.DisplayName, &u.Status, &u.Version, &u.CreatedAt, &u.DeletedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if e != nil {
		return User{}, fmt.Errorf("look up user by email: %w", e)
	}
	return u, nil
}

// StartSession opens a session for an account that already exists. The returned token is only
// carried by the session cookie; PostgreSQL keeps its hash.
func (s *Store) StartSession(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	token := make([]byte, 32)
	if _, e := rand.Read(token); e != nil {
		return "", fmt.Errorf("session token: %w", e)
	}
	secret := base64.RawURLEncoding.EncodeToString(token)
	if _, e := s.pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, now() + $3::interval)`,
		hash(secret), userID, ttl.String()); e != nil {
		return "", fmt.Errorf("create session: %w", e)
	}
	return secret, nil
}
