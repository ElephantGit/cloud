package execution

import "context"

// ModelStartRequest starts a model-only conversation without Project, Workspace, Agent or tools.
// ModelID resolves through the same provider configuration used for Agent model grants.
type ModelStartRequest struct {
	Caller  Caller
	ModelID string
	Prompt  Prompt
}

// Model is a credential-free selection snapshot. BindingVersion identifies immutable provider
// configuration; ID is a catalog key, not an arbitrary client-supplied upstream model name.
type Model struct {
	ID             string
	Name           string
	BindingVersion string
}

// Executor identifies an installed Agent and model binding. Each model choice has an executor ID.
type Executor struct {
	ID            string
	Name          string
	AgentID       string
	AgentName     string
	PluginVersion string
	Model         Model
}

// Catalog supplies configured choices, including fixture entries, from the shared module.
// It cannot invent runnable entries for unavailable artifacts or disclose provider credentials.
type Catalog interface {
	ListExecutors(context.Context, Scope) ([]Executor, error)
	ListModels(context.Context, Scope) ([]Model, error)
}
