package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/wanglongan587/cloud/v2-server/internal/store"
	"github.com/wanglongan587/cloud/v2-server/internal/store/storetest"
)

func TestSkillsAreUserScopedAndImportOverwriteKeepsIdentity(t *testing.T) {
	s := storetest.Open(t)
	owner, tenant := storetest.Login(t, s, "skills-owner")
	memberToken := storetest.SignIn(t, s, "skills-member", "skills-member", store.RoleMember)
	member, err := s.SessionUser(t.Context(), memberToken)
	if err != nil {
		t.Fatal(err)
	}

	created, err := s.CreateSkill(t.Context(), tenant, owner, store.SkillInput{
		Name: "review", Description: "检查改动", Content: "# Review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Version != 1 || created.Content != "# Review" {
		t.Fatalf("created = %+v", created)
	}
	if _, err := s.CreateSkill(t.Context(), tenant, owner, store.SkillInput{
		Name: "REVIEW", Description: "重复", Content: "",
	}); !errors.Is(err, store.ErrSkillNameConflict) {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := s.ListSkills(t.Context(), tenant, member.ID); err != nil {
		t.Fatal(err)
	}
	memberSkills, err := s.ListSkills(t.Context(), tenant, member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(memberSkills) != 3 {
		t.Fatalf("member starter skills = %+v", memberSkills)
	}
	for _, item := range memberSkills {
		if item.Name == "review" {
			t.Fatalf("member sees another user's skill: %+v", item)
		}
	}

	imported, err := s.ImportSkill(t.Context(), tenant, owner, []byte(
		"---\nname: deploy-checklist\ndescription: 部署检查\n---\n\n# Checklist\n",
	), false)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Name != "deploy-checklist" || imported.Content != "# Checklist\n" {
		t.Fatalf("imported = %+v", imported)
	}
	overwritten, err := s.ImportSkill(t.Context(), tenant, owner, []byte(
		"---\nname: review\ndescription: 更新后的说明\n---\n\n# Review v2\n",
	), false)
	if !errors.Is(err, store.ErrSkillNameConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	overwritten, err = s.ImportSkill(t.Context(), tenant, owner, []byte(
		"---\nname: review\ndescription: 更新后的说明\n---\n\n# Review v2\n",
	), true)
	if err != nil {
		t.Fatal(err)
	}
	if overwritten.ID != created.ID || overwritten.Version != 2 || overwritten.Description != "更新后的说明" {
		t.Fatalf("overwritten = %+v, original = %+v", overwritten, created)
	}

	docs, err := s.UserSkillDocuments(t.Context(), tenant, owner)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, item := range docs {
		names[item.Name] = true
	}
	if len(docs) != 5 || !names["deploy-checklist"] || !names["review"] ||
		!names["ora-code-review"] || !names["test-plan"] || !names["release-notes"] {
		t.Fatalf("runtime documents = %+v", docs)
	}
	if err := s.DeleteSkill(t.Context(), tenant, owner, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSkill(t.Context(), tenant, owner, created.ID); !errors.Is(err, store.ErrSkillNotFound) {
		t.Fatalf("deleted skill error = %v", err)
	}
}

func TestSkillsRejectCallersOutsideTheWorkspace(t *testing.T) {
	s := storetest.Open(t)
	storetest.Login(t, s, "skills-scope-owner")
	token := storetest.SignIn(t, s, "skills-outsider", "outsider", store.RoleMember)
	outsider, err := s.SessionUser(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	tenant := "00000000-0000-0000-0000-000000000001"
	if _, err := s.ListSkills(t.Context(), tenant, outsider.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("list error = %v", err)
	}
	if _, err := s.CreateSkill(t.Context(), tenant, outsider.ID, store.SkillInput{
		Name: "review", Description: "检查", Content: "body",
	}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("create error = %v", err)
	}
}

func TestSkillUpdatesUseServerTimeAndNamesRemainCaseInsensitive(t *testing.T) {
	s := storetest.Open(t)
	owner, tenant := storetest.Login(t, s, "skills-time-owner")
	created, err := s.CreateSkill(t.Context(), tenant, owner, store.SkillInput{
		Name: "review", Description: "检查", Content: "body",
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	updated, err := s.UpdateSkill(t.Context(), tenant, owner, created.ID, store.SkillInput{
		Name: "REVIEW", Description: "更新", Content: "body 2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || !updated.UpdatedAt.After(created.UpdatedAt) || updated.Name != "REVIEW" {
		t.Fatalf("updated = %+v, created = %+v", updated, created)
	}
}

func TestStarterSkillsSeedOnceAndStayDeleted(t *testing.T) {
	s := storetest.Open(t)
	owner, tenant := storetest.Login(t, s, "skills-starter-owner")
	items, err := s.ListSkills(t.Context(), tenant, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("starter skills = %+v", items)
	}
	var review string
	for _, item := range items {
		if item.Name == "ora-code-review" {
			review = item.ID
		}
	}
	if review == "" {
		t.Fatalf("ora-code-review missing from %+v", items)
	}
	if err := s.DeleteSkill(t.Context(), tenant, owner, review); err != nil {
		t.Fatal(err)
	}
	storetest.SignIn(t, s, "skills-starter-owner", "skills-starter-owner", store.RoleOwner)
	items, err = s.ListSkills(t.Context(), tenant, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("deleted starter returned: %+v", items)
	}
}

func TestStarterSkillNameMigration(t *testing.T) {
	seed := func(t *testing.T) (*store.Store, string, string, store.Skill) {
		t.Helper()
		s := storetest.Open(t)
		owner, tenant := storetest.Login(t, s, "starter-migration-"+t.Name())
		items, err := s.ListSkills(t.Context(), tenant, owner)
		if err != nil {
			t.Fatal(err)
		}
		review, ok := skillNamed(items, "ora-code-review")
		if !ok {
			t.Fatalf("starter = %+v, want ora-code-review", items)
		}
		return s, owner, tenant, review
	}

	t.Run("unedited starter is renamed once", func(t *testing.T) {
		s, owner, tenant, review := seed(t)
		if _, err := s.UpdateSkill(t.Context(), tenant, owner, review.ID, store.SkillInput{
			Name: "code-review", Description: review.Description, Content: review.Content,
		}); err != nil {
			t.Fatal(err)
		}
		if err := s.MigrateStarterSkillNames(t.Context()); err != nil {
			t.Fatal(err)
		}
		items, err := s.ListSkills(t.Context(), tenant, owner)
		if err != nil {
			t.Fatal(err)
		}
		if _, old := skillNamed(items, "code-review"); old {
			t.Fatalf("legacy starter still live: %+v", items)
		}
		if _, renamed := skillNamed(items, "ora-code-review"); !renamed {
			t.Fatalf("renamed starter missing: %+v", items)
		}
		if err := s.MigrateStarterSkillNames(t.Context()); err != nil {
			t.Fatal(err)
		}
		again, err := s.ListSkills(t.Context(), tenant, owner)
		if err != nil || len(again) != len(items) {
			t.Fatalf("idempotent migration = %+v, %v", again, err)
		}
	})

	t.Run("edited starter is not renamed", func(t *testing.T) {
		s, owner, tenant, review := seed(t)
		if _, err := s.UpdateSkill(t.Context(), tenant, owner, review.ID, store.SkillInput{
			Name: "code-review", Description: review.Description, Content: "# edited\n",
		}); err != nil {
			t.Fatal(err)
		}
		if err := s.MigrateStarterSkillNames(t.Context()); err != nil {
			t.Fatal(err)
		}
		items, err := s.ListSkills(t.Context(), tenant, owner)
		if err != nil {
			t.Fatal(err)
		}
		if _, old := skillNamed(items, "code-review"); !old {
			t.Fatalf("edited starter was renamed: %+v", items)
		}
		if _, target := skillNamed(items, "ora-code-review"); target {
			t.Fatalf("edited starter gained target name: %+v", items)
		}
	})

	t.Run("deleted starter is not recreated", func(t *testing.T) {
		s, owner, tenant, review := seed(t)
		if err := s.DeleteSkill(t.Context(), tenant, owner, review.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.MigrateStarterSkillNames(t.Context()); err != nil {
			t.Fatal(err)
		}
		items, err := s.ListSkills(t.Context(), tenant, owner)
		if err != nil {
			t.Fatal(err)
		}
		if _, old := skillNamed(items, "code-review"); old {
			t.Fatalf("deleted starter came back as code-review: %+v", items)
		}
		if _, target := skillNamed(items, "ora-code-review"); target {
			t.Fatalf("deleted starter came back under target name: %+v", items)
		}
	})

	t.Run("user-created same name is not renamed", func(t *testing.T) {
		s, owner, tenant, review := seed(t)
		if err := s.DeleteSkill(t.Context(), tenant, owner, review.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateSkill(t.Context(), tenant, owner, store.SkillInput{
			Name: "code-review", Description: "用户自己创建", Content: "# my review\n",
		}); err != nil {
			t.Fatal(err)
		}
		if err := s.MigrateStarterSkillNames(t.Context()); err != nil {
			t.Fatal(err)
		}
		items, err := s.ListSkills(t.Context(), tenant, owner)
		if err != nil {
			t.Fatal(err)
		}
		if item, ok := skillNamed(items, "code-review"); !ok || item.Description != "用户自己创建" {
			t.Fatalf("user-created code-review changed: %+v", items)
		}
		if _, target := skillNamed(items, "ora-code-review"); target {
			t.Fatalf("user-created code-review gained target name: %+v", items)
		}
	})

	t.Run("occupied target is left for the user to resolve", func(t *testing.T) {
		s, owner, tenant, review := seed(t)
		if _, err := s.UpdateSkill(t.Context(), tenant, owner, review.ID, store.SkillInput{
			Name: "code-review", Description: review.Description, Content: review.Content,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateSkill(t.Context(), tenant, owner, store.SkillInput{
			Name: "ora-code-review", Description: "自己的技能", Content: "# mine\n",
		}); err != nil {
			t.Fatal(err)
		}
		if err := s.MigrateStarterSkillNames(t.Context()); err != nil {
			t.Fatal(err)
		}
		items, err := s.ListSkills(t.Context(), tenant, owner)
		if err != nil {
			t.Fatal(err)
		}
		if _, old := skillNamed(items, "code-review"); !old {
			t.Fatalf("legacy starter disappeared: %+v", items)
		}
		if target, ok := skillNamed(items, "ora-code-review"); !ok || target.Content != "# mine\n" {
			t.Fatalf("occupied target was overwritten: %+v", items)
		}
	})
}

func skillNamed(items []store.Skill, name string) (store.Skill, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return store.Skill{}, false
}
