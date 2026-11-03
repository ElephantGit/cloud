import type { InstalledPlugin } from "../contracts";

/**
 * Demo MCP server packages. Roles come from the user's Settings snapshot, Skills from the
 * server-backed user catalog, and executors/models from the real execution catalog.
 */
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
