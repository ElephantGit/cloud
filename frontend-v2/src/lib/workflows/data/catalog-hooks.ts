import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import type { Skill } from "../contracts";
import type { WorkflowAgentModel } from "../mock";
import { useRoleCatalog } from "@/lib/settings-demo/role-hooks";
import { useContractsClient } from "./contracts-client";
import { useAgentCatalog, type AgentEntry } from "./use-agent-catalog";

// Desktop's catalog hooks (`state/hooks/use-agents`, `use-skills`, `use-installed-plugins`,
// `use-workflow-agent-models`) over the local client. Model discovery needs no Workspace here, so
// the desktop selection plumbing that picks one is left out.

const catalogKeys = {
  skills: ["workflow-catalog", "skills"] as const,
  plugins: ["workflow-catalog", "plugins"] as const,
  executors: ["workflow-catalog", "executors"] as const,
};

/** Loads configurable roles from the same browser-local snapshot Settings edits. */
export function useAgents() {
  const query = useRoleCatalog();
  return {
    ...query,
    data: query.data?.map((role) => ({
      id: role.id,
      namespace: role.namespace,
      name: role.name,
      description: role.description,
    })),
  };
}

/** Pickers only surface skills whose on-disk package is still usable. */
export function availableSkills(skills: readonly Skill[]): Skill[] {
  return skills.filter((skill) => skill.availability === "available");
}

/** Loads configurable skills. */
export function useSkills() {
  const client = useContractsClient();
  return useQuery({
    queryKey: [...catalogKeys.skills, client.tenantId],
    queryFn: () => client.skill.list({}).then((response) => response.skills),
  });
}

/** Loads MCP packages for the workflow's existing dependency picker. */
export function useInstalledPlugins() {
  const client = useContractsClient();
  return useQuery({
    queryKey: [...catalogKeys.plugins, client.tenantId],
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

/** Reads the same executable agent/model bindings as Chat; failures never supply demo data. */
export function useWorkflowExecutors() {
  const client = useContractsClient();
  return useQuery({
    queryKey: [...catalogKeys.executors, client.tenantId],
    queryFn: () => client.executors.list(),
  });
}

/** Builds the Agent picker from one authoritative executor directory. */
export function useWorkflowAgentModels(): WorkflowAgentModelsCatalog {
  const query = useWorkflowExecutors();
  const agents = useAgentCatalog();
  const agentModels = useMemo(
    () => (query.data?.items ?? []).map((executor) => ({
      agentCli: executor.agentId,
      modelId: executor.modelId,
      label: `${executor.agentName} · ${executor.modelName}`,
    })),
    [query.data],
  );
  const modelsByCli = useMemo(() => {
    const byCli = new Map<string, WorkflowAgentModel[]>();
    for (const model of agentModels) {
      byCli.set(model.agentCli, [...(byCli.get(model.agentCli) ?? []), model]);
    }
    return byCli;
  }, [agentModels]);
  const cliStatus = Object.fromEntries(agents.map((agent) => [agent.agentRef, {
    isLoading: query.isPending,
    isError: query.isError,
  }]));
  return {
    agents,
    agentModels,
    modelsByCli,
    cliStatus,
    isLoading: query.isPending,
    isError: query.isError,
    refetch: () => {
      void query.refetch();
    },
  };
}
