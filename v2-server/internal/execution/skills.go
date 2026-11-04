package execution

// Skills freezes the imported skills selected for a session. Nil preserves ordinary Chat
// discovery; a non-nil empty selection explicitly exposes no imported workflow skills.
// Repository-authored skills remain project files, independent of these imports.
type Skills struct {
	Documents []SkillDocument
	Plugins   []SkillPlugin
}

// SkillDocument is the caller-owned content frozen before session admission.
type SkillDocument struct{ Name, Description, Content string }

// SkillPlugin identifies an installed package version in the target project's space.
type SkillPlugin struct{ ID, Version string }
