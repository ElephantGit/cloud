package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrAlreadyMember reports an invitation for an address that already joined the workspace.
var ErrAlreadyMember = errors.New("already a member")

// ErrSquadNotFound reports an invitation into a squad the workspace does not have live.
var ErrSquadNotFound = errors.New("squad not found")

// Invite is one owner-initiated membership: the address to join, an optional display name that
// defaults to the address's local part, and an optional squad the new member joins at once.
type Invite struct {
	Email       string
	DisplayName string
	SquadID     string
}

// InviteMember creates the account when the address is new, joins it to the workspace as a
// member, and optionally to one live squad. Only an owner may invite; anyone else is ErrForbidden.
// The account is created in the same transaction as the membership so a failure leaves neither.
func (s *Store) InviteMember(ctx context.Context, tenant, caller string, in Invite) (Member, error) {
	email, e := NormalizeEmail(in.Email)
	if e != nil {
		return Member{}, e
	}
	display := in.DisplayName
	if display == "" {
		display = emailLocalPart(email)
	}
	var m Member
	e = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		role, e := tenantRole(ctx, tx, tenant, caller)
		if e != nil {
			return e
		}
		if role != RoleOwner {
			return ErrForbidden
		}
		userID, e := invitee(ctx, tx, email, display)
		if e != nil {
			return e
		}
		var joined bool
		if e := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM memberships WHERE tenant_id = $1 AND user_id = $2)`,
			tenant, userID).Scan(&joined); e != nil {
			return fmt.Errorf("look up membership: %w", e)
		}
		if joined {
			return ErrAlreadyMember
		}
		// Two owners may invite the same address at once; the loser sees the winner's row.
		tag, e := tx.Exec(ctx, `
			INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, tenant, userID, RoleMember)
		if e != nil {
			return fmt.Errorf("join invited member: %w", e)
		}
		if tag.RowsAffected() == 0 {
			return ErrAlreadyMember
		}
		if e := seedStarterSkills(ctx, tx, tenant, userID); e != nil {
			return e
		}
		if in.SquadID != "" {
			if e := joinInvitedSquad(ctx, tx, tenant, in.SquadID, userID); e != nil {
				return e
			}
		}
		if e := tx.QueryRow(ctx, `
			SELECT u.id, u.display_name, m.role, u.status
			FROM memberships m JOIN users u ON u.id = m.user_id
			WHERE m.tenant_id = $1 AND m.user_id = $2`, tenant, userID).
			Scan(&m.UserID, &m.DisplayName, &m.Role, &m.Status); e != nil {
			return fmt.Errorf("read invited member: %w", e)
		}
		return nil
	})
	if e != nil {
		return Member{}, e
	}
	m.ID, m.TenantID, m.Version = m.UserID, tenant, 1
	return m, nil
}

// invitee resolves the account an invitation targets, creating it for a new address. An address
// that exists but was deleted stays unavailable: v2 never resurrects a removed account.
func invitee(ctx context.Context, tx pgx.Tx, email, display string) (string, error) {
	var userID string
	var deleted bool
	e := tx.QueryRow(ctx, `
		SELECT id, deleted_at IS NOT NULL FROM users
		WHERE source = $1 AND subject = $2 FOR UPDATE`, EmailSource, email).Scan(&userID, &deleted)
	if errors.Is(e, pgx.ErrNoRows) {
		// FOR UPDATE cannot lock a row that does not exist yet, so a concurrent invite of the same
		// new address falls through to the unique constraint; it then reads the created account.
		if _, e := tx.Exec(ctx, `
			INSERT INTO users (source, subject, display_name) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, EmailSource, email, display); e != nil {
			return "", fmt.Errorf("create invited user: %w", e)
		}
		if e := tx.QueryRow(ctx, `SELECT id FROM users WHERE source = $1 AND subject = $2`,
			EmailSource, email).Scan(&userID); e != nil {
			return "", fmt.Errorf("read invited user: %w", e)
		}
		return userID, nil
	}
	if e != nil {
		return "", fmt.Errorf("look up invited user: %w", e)
	}
	if deleted {
		return "", ErrUserUnavailable
	}
	return userID, nil
}

// joinInvitedSquad adds the new member to a live squad of the same workspace. An archived or
// foreign squad is ErrSquadNotFound so the invitation cannot reveal it.
func joinInvitedSquad(ctx context.Context, tx pgx.Tx, tenant, squad, user string) error {
	var live bool
	if e := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM v2_squads
			WHERE id = $2 AND tenant_id = $1 AND archived_at IS NULL)`,
		tenant, squad).Scan(&live); e != nil {
		return fmt.Errorf("look up squad: %w", e)
	}
	if !live {
		return ErrSquadNotFound
	}
	tag, e := tx.Exec(ctx, `
		INSERT INTO v2_squad_members (squad_id, member_type, user_id, role, maintainer)
		VALUES ($1, $2, $3, '', false)
		ON CONFLICT DO NOTHING`, squad, MemberHuman, user)
	if e != nil {
		return fmt.Errorf("join invited squad: %w", e)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	// A squad edit read before this invite must see its version move, exactly like a manual add.
	return bumpSquad(ctx, tx, squad)
}
