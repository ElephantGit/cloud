package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/wanglongan587/cloud/v2-server/internal/skilldoc"
)

// Skill is one user-owned catalog entry together with its Markdown body.
type Skill struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// SkillInput is a validated create or replacement payload.
type SkillInput struct {
	Name        string
	Description string
	Content     string
}

// SkillDocument is the subset materialized into a project checkout.
type SkillDocument struct {
	Name        string
	Description string
	Content     string
}

// ErrSkillNameConflict reports a live skill with the same case-insensitive name.
var ErrSkillNameConflict = errors.New("skill name conflict")

// ErrSkillNotFound reports a skill outside the caller's visible catalog.
var ErrSkillNotFound = errors.New("skill not found")

// ListSkills returns the caller's live skills, newest update first.
func (s *Store) ListSkills(ctx context.Context, tenant, caller string) ([]Skill, error) {
	if _, e := tenantRole(ctx, s.pool, tenant, caller); e != nil {
		return nil, e
	}
	rows, e := s.pool.Query(ctx, `
		SELECT id::text, name, description, content, version, created_at, updated_at
		FROM v2_skills
		WHERE tenant_id = $1 AND user_id = $2 AND deleted_at IS NULL
		ORDER BY updated_at DESC, id`, tenant, caller)
	if e != nil {
		return nil, fmt.Errorf("list skills: %w", e)
	}
	defer rows.Close()
	items := make([]Skill, 0)
	for rows.Next() {
		var item Skill
		if e := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Content, &item.Version, &item.CreatedAt, &item.UpdatedAt); e != nil {
			return nil, fmt.Errorf("scan skill: %w", e)
		}
		items = append(items, item)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("iterate skills: %w", e)
	}
	return items, nil
}

// GetSkill returns one skill owned by the caller.
func (s *Store) GetSkill(ctx context.Context, tenant, caller, id string) (Skill, error) {
	if _, e := tenantRole(ctx, s.pool, tenant, caller); e != nil {
		return Skill{}, e
	}
	item, e := querySkill(ctx, s.pool, tenant, caller, id)
	if errors.Is(e, pgx.ErrNoRows) {
		return Skill{}, ErrSkillNotFound
	}
	if e != nil {
		return Skill{}, fmt.Errorf("get skill: %w", e)
	}
	return item, nil
}

// CreateSkill persists one new user-owned skill.
func (s *Store) CreateSkill(ctx context.Context, tenant, caller string, in SkillInput) (Skill, error) {
	doc, e := skilldoc.Validate(skilldoc.Document{Name: in.Name, Description: in.Description, Content: in.Content})
	if e != nil {
		return Skill{}, e
	}
	if _, e := tenantRole(ctx, s.pool, tenant, caller); e != nil {
		return Skill{}, e
	}
	var item Skill
	e = s.pool.QueryRow(ctx, `
		INSERT INTO v2_skills (tenant_id, user_id, name, description, content)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, name, description, content, version, created_at, updated_at`,
		tenant, caller, doc.Name, doc.Description, doc.Content).
		Scan(&item.ID, &item.Name, &item.Description, &item.Content, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	if uniqueViolation(e) {
		return Skill{}, ErrSkillNameConflict
	}
	if e != nil {
		return Skill{}, fmt.Errorf("create skill: %w", e)
	}
	return item, nil
}

// UpdateSkill replaces one owned skill while preserving its identity.
func (s *Store) UpdateSkill(ctx context.Context, tenant, caller, id string, in SkillInput) (Skill, error) {
	doc, e := skilldoc.Validate(skilldoc.Document{Name: in.Name, Description: in.Description, Content: in.Content})
	if e != nil {
		return Skill{}, e
	}
	if _, e := tenantRole(ctx, s.pool, tenant, caller); e != nil {
		return Skill{}, e
	}
	var item Skill
	e = s.pool.QueryRow(ctx, `
		UPDATE v2_skills
		SET name = $1, description = $2, content = $3, version = version + 1, updated_at = now()
		WHERE id = $4 AND tenant_id = $5 AND user_id = $6 AND deleted_at IS NULL
		RETURNING id::text, name, description, content, version, created_at, updated_at`,
		doc.Name, doc.Description, doc.Content, id, tenant, caller).
		Scan(&item.ID, &item.Name, &item.Description, &item.Content, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	if uniqueViolation(e) {
		return Skill{}, ErrSkillNameConflict
	}
	if errors.Is(e, pgx.ErrNoRows) {
		return Skill{}, ErrSkillNotFound
	}
	if e != nil {
		return Skill{}, fmt.Errorf("update skill: %w", e)
	}
	return item, nil
}

// DeleteSkill soft-deletes one owned skill and frees its name for a later import or create.
func (s *Store) DeleteSkill(ctx context.Context, tenant, caller, id string) error {
	if _, e := tenantRole(ctx, s.pool, tenant, caller); e != nil {
		return e
	}
	tag, e := s.pool.Exec(ctx, `
		UPDATE v2_skills
		SET deleted_at = now(), updated_at = now(), version = version + 1
		WHERE id = $1 AND tenant_id = $2 AND user_id = $3 AND deleted_at IS NULL`, id, tenant, caller)
	if e != nil {
		return fmt.Errorf("delete skill: %w", e)
	}
	if tag.RowsAffected() == 0 {
		return ErrSkillNotFound
	}
	return nil
}

// ImportSkill parses one SKILL.md and creates or explicitly overwrites the caller's entry.
func (s *Store) ImportSkill(ctx context.Context, tenant, caller string, raw []byte, overwrite bool) (Skill, error) {
	doc, e := skilldoc.Parse(raw)
	if e != nil {
		return Skill{}, e
	}
	if _, e := tenantRole(ctx, s.pool, tenant, caller); e != nil {
		return Skill{}, e
	}
	existing, e := querySkillByName(ctx, s.pool, tenant, caller, doc.Name)
	switch {
	case e == nil:
		if !overwrite {
			return Skill{}, ErrSkillNameConflict
		}
		return s.UpdateSkill(ctx, tenant, caller, existing.ID, SkillInput{
			Name: doc.Name, Description: doc.Description, Content: doc.Content,
		})
	case errors.Is(e, pgx.ErrNoRows):
		return s.CreateSkill(ctx, tenant, caller, SkillInput{
			Name: doc.Name, Description: doc.Description, Content: doc.Content,
		})
	default:
		return Skill{}, fmt.Errorf("look up imported skill: %w", e)
	}
}

// UserSkillDocuments returns one active member's live skills for workspace materialization.
func (s *Store) UserSkillDocuments(ctx context.Context, tenant, user string) ([]SkillDocument, error) {
	if _, e := tenantRole(ctx, s.pool, tenant, user); e != nil {
		return nil, e
	}
	rows, e := s.pool.Query(ctx, `
		SELECT name, description, content
		FROM v2_skills
		WHERE tenant_id = $1 AND user_id = $2 AND deleted_at IS NULL
		ORDER BY name`, tenant, user)
	if e != nil {
		return nil, fmt.Errorf("list skill documents: %w", e)
	}
	defer rows.Close()
	items := make([]SkillDocument, 0)
	for rows.Next() {
		var item SkillDocument
		if e := rows.Scan(&item.Name, &item.Description, &item.Content); e != nil {
			return nil, fmt.Errorf("scan skill document: %w", e)
		}
		items = append(items, item)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("iterate skill documents: %w", e)
	}
	return items, nil
}

func querySkill(ctx context.Context, q dbTX, tenant, user, id string) (Skill, error) {
	var item Skill
	e := q.QueryRow(ctx, `
		SELECT id::text, name, description, content, version, created_at, updated_at
		FROM v2_skills
		WHERE id = $1 AND tenant_id = $2 AND user_id = $3 AND deleted_at IS NULL`, id, tenant, user).
		Scan(&item.ID, &item.Name, &item.Description, &item.Content, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	return item, e
}

func querySkillByName(ctx context.Context, q dbTX, tenant, user, name string) (Skill, error) {
	var item Skill
	e := q.QueryRow(ctx, `
		SELECT id::text, name, description, content, version, created_at, updated_at
		FROM v2_skills
		WHERE tenant_id = $1 AND user_id = $2 AND lower(name) = lower($3) AND deleted_at IS NULL`, tenant, user, name).
		Scan(&item.ID, &item.Name, &item.Description, &item.Content, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	return item, e
}

func uniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
