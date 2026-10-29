import { createContext, useContext } from "react";
import { LocalWorkflowBackend } from "./local-workflow-backend";
import {
  demoAgentModels,
  demoPlugins,
  demoRoles,
  demoSkills,
} from "./demo-catalog";

/**
 * The part of desktop's contracts client the workflow page calls. Every operation answers
 * asynchronously, as a network call would, so the copied query and mutation hooks behave the same.
 */
export function createLocalContractsClient(
  backend: LocalWorkflowBackend = new LocalWorkflowBackend(),
) {
  const call =
    <Request, Response>(operation: (request: Request) => Response) =>
    async (request: Request, _options?: { signal?: AbortSignal }) =>
      operation(request);
  return {
    workflow: {
      activate: call(backend.activate.bind(backend)),
      analyze: call(backend.analyze.bind(backend)),
      create: call(backend.create.bind(backend)),
      delete: call(backend.delete.bind(backend)),
      deleteSnapshot: call(backend.deleteSnapshot.bind(backend)),
      get: call(backend.get.bind(backend)),
      getDraft: call(backend.getDraft.bind(backend)),
      getSnapshot: call(backend.getSnapshot.bind(backend)),
      getVersion: call(backend.getVersion.bind(backend)),
      list: call((_request: Record<string, never>) => backend.list()),
      listVersions: call(backend.listVersions.bind(backend)),
      publish: call(backend.publish.bind(backend)),
      rollback: call(backend.rollback.bind(backend)),
      update: call(backend.update.bind(backend)),
      updateDraft: call(backend.updateDraft.bind(backend)),
    },
    agent: {
      list: call((_request: Record<string, never>) => ({ agents: demoRoles })),
    },
    skill: {
      list: call((_request: Record<string, never>) => ({ skills: demoSkills })),
    },
    plugin: {
      listInstalled: call((_request: Record<string, never>) => ({
        plugins: demoPlugins,
      })),
    },
    agentRuntime: {
      listModels: call((request: { agentRef: string }) => ({
        models: demoAgentModels[request.agentRef] ?? [],
      })),
    },
  };
}

export type LocalContractsClient = ReturnType<typeof createLocalContractsClient>;

export const ContractsClientContext = createContext<LocalContractsClient | null>(null);

export function useContractsClient(): LocalContractsClient {
  const client = useContext(ContractsClientContext);
  if (client === null) {
    throw new Error("useContractsClient must be used inside ContractsClientProvider");
  }
  return client;
}
