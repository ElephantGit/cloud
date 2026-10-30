import type { InstalledPlugin, Skill } from "../contracts";

/**
 * Demo stand-ins for the catalogs desktop reads from its backend: configurable Roles, installed
 * Skills, installed plugin packages (Agents and MCP servers) and each Agent's models. v2-server
 * does not report any of these yet; the Agent node's pickers show these entries until it does.
 * Model ids are placeholders, not a claim that the model is reachable.
 */
export interface DemoRole {
  id: string;
  name: string;
  /** Shown under the role in a run's Agent configuration. */
  description?: string;
}

export const demoRoles: DemoRole[] = [
  { id: "role-reviewer", name: "代码审查", description: "检查改动的正确性、风险和测试覆盖" },
  { id: "role-writer", name: "技术写作", description: "把结论整理成清楚的说明文档" },
];

export const demoSkills: Skill[] = [
  {
    id: "skill-summarize",
    namespace: "local",
    name: "summarize",
    description: "把长文本整理成要点",
    source: { kind: "local" },
    availability: "available",
  },
  {
    id: "skill-test-plan",
    namespace: "local",
    name: "test-plan",
    description: "为改动列出测试计划",
    source: { kind: "local" },
    availability: "available",
  },
];

function plugin(
  id: string,
  displayName: string,
  kind: { kind: "agent"; agentDisplayName: string } | { kind: "mcp" },
): InstalledPlugin {
  const [namespace, name] = id.split("/") as [string, string];
  return {
    id,
    namespace,
    name,
    displayName,
    version: "0.0.0",
    description: "",
    homepage: null,
    license: null,
    logo: null,
    installationValidity: { validity: "valid" },
    configuration: { state: "not_declared" },
    runtime: "running",
    ...kind,
  };
}

export const demoPlugins: InstalledPlugin[] = [
  plugin("ora/claude-code", "Claude Code", {
    kind: "agent",
    agentDisplayName: "Claude Code",
  }),
  plugin("ora/codex", "Codex", { kind: "agent", agentDisplayName: "Codex" }),
  plugin("ora/opencode", "OpenCode", {
    kind: "agent",
    agentDisplayName: "OpenCode",
  }),
  plugin("ora/github-mcp", "GitHub MCP", { kind: "mcp" }),
  plugin("ora/filesystem-mcp", "Filesystem MCP", { kind: "mcp" }),
];

/** Models each demo Agent offers, keyed by the Agent's plugin id. */
export const demoAgentModels: Record<
  string,
  { id: string; displayName: string }[]
> = {
  "ora/claude-code": [
    { id: "claude-sonnet-5-5", displayName: "Sonnet 5.5" },
    { id: "claude-opus-5-5", displayName: "Opus 5.5" },
  ],
  "ora/codex": [{ id: "codex-default", displayName: "默认模型" }],
  "ora/opencode": [{ id: "opencode-default", displayName: "默认模型" }],
};
