import type { InstalledPlugin, Skill } from "../contracts";

/**
 * Demo stand-ins for the catalogs desktop reads from its backend: configurable Roles, installed
 * Skills and MCP server packages. Agent executors and models use the real execution catalog.
 * v2-server does not report Roles, Skills or MCP catalogs yet; those pickers retain these entries.
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
  plugin("ora/github-mcp", "GitHub MCP", { kind: "mcp" }),
  plugin("ora/filesystem-mcp", "Filesystem MCP", { kind: "mcp" }),
];
