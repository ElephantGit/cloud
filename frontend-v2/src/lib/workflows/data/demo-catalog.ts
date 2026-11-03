import type { InstalledPlugin, Skill } from "../contracts";

/**
 * Demo stand-ins for the catalogs desktop reads from its backend: installed Skills and MCP server
 * packages. Roles come from the user's Settings snapshot; executors and models use the real
 * execution catalog. v2-server does not report Skills or MCP catalogs yet.
 */
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
