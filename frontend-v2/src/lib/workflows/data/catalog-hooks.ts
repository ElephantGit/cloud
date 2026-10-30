import { useMemo } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import type { Skill } from "../contracts";
import type { WorkflowAgentModel } from "../mock";
import { useContractsClient } from "./contracts-client";
import { useAgentCatalog, type AgentEntry } from "./use-agent-catalog";

// Desktop's catalog hooks (`state/hooks/use-agents`, `use-skills`, `use-installed-plugins`,
// `use-workflow-agent-models`) over the local client. Model discovery needs no Workspace here, so
// the desktop selection plumbing that picks one is left out.

const catalogKeys = {
  agents: ["workflow-catalog", "agents"] as const,
  skills: ["workflow-catalog", "skills"] as const,
  plugins: ["workflow-catalog", "plugins"] as const,
  models: (agentRef: string) =>
    ["workflow-catalog", "models", agentRef] as const,
};

/** Loads configurable agents (Roles). */
export function useAgents() {
  const client = useContractsClient();
  return useQuery({
    queryKey: catalogKeys.agents,
    queryFn: () => client.agent.list({}).then((response) => response.agents),
  });
}

/** Pickers only surface skills whose on-disk package is still usable. */
export function availableSkills(skills: readonly Skill[]): Skill[] {
  return skills.filter((skill) => skill.availability === "available");
}

/** Loads configurable skills. */
export function useSkills() {
  const client = useContractsClient();
  return useQuery({
    queryKey: catalogKeys.skills,
    queryFn: () => client.skill.list({}).then((response) => response.skills),
  });
}

/** Loads the installed plugin packages: Agents and MCP servers. */
export function useInstalledPlugins() {
  const client = useContractsClient();
  return useQuery({
    queryKey: catalogKeys.plugins,
    queryFn: () =>
      client.plugin.listInstalled({}).then((response) => response.plugins),
  });
}

export interface WorkflowAgentCliStatus {
  isLoading: boolean;
  isError: boolean;
}

export interface WorkflowAgentModelsCatalog {
  agents: AgentEntry[];
  agentModels: WorkflowAgentModel[];
  modelsByCli: ReadonlyMap<string, WorkflowAgentModel[]>;
  cliStatus: Readonly<Record<string, WorkflowAgentCliStatus>>;
  isLoading: boolean;
  isError: boolean;
  refetch: () => void;
}

/** Discovers every installed Agent's models for the Agent node's executor picker. */
export function useWorkflowAgentModels(): WorkflowAgentModelsCatalog {
  const client = useContractsClient();
  const agents = useAgentCatalog();
  const modelQueries = useQueries({
    queries: agents.map((agent) => ({
      queryKey: catalogKeys.models(agent.agentRef),
      queryFn: () =>
        client.agentRuntime.listModels({ agentRef: agent.agentRef }),
      retry: false,
    })),
  });

  const agentModels = useMemo(() => {
    const models: WorkflowAgentModel[] = [];
    for (const [index, agent] of agents.entries()) {
      for (const model of modelQueries[index]?.data?.models ?? []) {
        models.push({
          agentCli: agent.agentRef,
          modelId: model.id,
          label: `${agent.label} · ${model.displayName}`,
        });
      }
    }
    return models;
  }, [agents, modelQueries]);

  const modelsByCli = useMemo(() => {
    const byCli = new Map<string, WorkflowAgentModel[]>();
    for (const model of agentModels) {
      byCli.set(model.agentCli, [...(byCli.get(model.agentCli) ?? []), model]);
    }
    return byCli;
  }, [agentModels]);

  const cliStatus = useMemo(
    () =>
      Object.fromEntries(
        agents.map((agent, index) => [
          agent.agentRef,
          {
            isLoading: modelQueries[index]?.isPending === true,
            isError: modelQueries[index]?.isError === true,
          },
        ]),
      ),
    [agents, modelQueries],
  );

  return {
    agents,
    agentModels,
    modelsByCli,
    cliStatus,
    isLoading: modelQueries.some((query) => query.isPending),
    isError: modelQueries.some((query) => query.isError),
    refetch: () => {
      for (const query of modelQueries) void query.refetch();
    },
  };
}
