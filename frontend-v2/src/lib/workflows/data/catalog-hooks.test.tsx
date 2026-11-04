import { afterEach, describe, expect, it } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { UserContext } from "@/app/auth/user";
import { http } from "@/lib/api-client";
import { demoUser, fakeBackend } from "@/test/backend";
import { useAgents, useWorkflowAgentModels } from "./catalog-hooks";
import { createContractsClient, createHttpWorkflowContracts } from "./contracts-client";
import { ContractsClientProvider } from "./contracts-client-provider";

const tid = "00000000-0000-4000-8000-0000000000aa";
const originalAdapter = http.defaults.adapter;
const executors = [
  { id: "oc:fixture", name: "OpenCode / Fixture", agentId: "official/ora-space.opencode", agentName: "OpenCode", modelId: "fixture", modelName: "Fixture" },
  { id: "oc:configured", name: "OpenCode / Real", agentId: "official/ora-space.opencode", agentName: "OpenCode", modelId: "configured", modelName: "Real" },
  { id: "cc:fixture", name: "Claude Code / Fixture", agentId: "official/ora-space.claude", agentName: "Claude Code", modelId: "messages-fixture", modelName: "Fixture" },
];

function mount() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const client = createContractsClient(createHttpWorkflowContracts(tid), tid);
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <ContractsClientProvider client={client}>{children}</ContractsClientProvider>
    </QueryClientProvider>
  );
  return { ...renderHook(() => useWorkflowAgentModels(), { wrapper }), client };
}

function mountAgents() {
  localStorage.clear();
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderHook(() => useAgents(), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>
        <UserContext.Provider value={demoUser}>{children}</UserContext.Provider>
      </QueryClientProvider>
    ),
  });
}

afterEach(() => { http.defaults.adapter = originalAdapter; });

describe("workflow executor catalog", () => {
  it("groups real executor identities and reads the real user skill catalog", async () => {
    const seen = fakeBackend((config) => config.url === `/api/v2/tenants/${tid}/chat/executors`
      ? { status: 200, data: { items: executors } }
      : config.url === `/api/v2/tenants/${tid}/skills`
        ? { status: 200, data: { items: [{
            id: "skill-review",
            name: "review",
            description: "检查改动",
            content: "# Review",
            version: 1,
            createdAt: "2026-11-02T08:00:00Z",
            updatedAt: "2026-11-02T08:00:00Z",
          }] } }
        : config.url === `/api/v1/tenants/${tid}/spaces`
          ? { status: 200, data: { items: [] } } : undefined);
    const { result, client } = mount();
    await waitFor(() => expect(result.current.agentModels).toHaveLength(3));
    expect(result.current.agents.map((a) => a.agentRef)).toEqual([
      "official/ora-space.opencode", "official/ora-space.claude",
    ]);
    expect(result.current.modelsByCli.get("official/ora-space.claude")).toEqual([
      { agentCli: "official/ora-space.claude", modelId: "messages-fixture", label: "Claude Code · Fixture" },
    ]);
    expect(seen).toEqual([`GET /api/v2/tenants/${tid}/chat/executors`]);
    expect((await client.skill.list({})).skills).toEqual([{
      id: "skill-review",
      namespace: "local",
      name: "review",
      description: "检查改动",
      source: { kind: "local" },
      availability: "available",
    }]);
    expect(seen).toContain(`GET /api/v2/tenants/${tid}/skills`);
    expect((await client.plugin.listInstalled({})).plugins).toEqual([]);
  });

  it("keeps an empty real directory empty", async () => {
    fakeBackend(() => ({ status: 200, data: { items: [] } }));
    const { result } = mount();
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.isError).toBe(false);
    expect(result.current.agents).toEqual([]);
    expect(result.current.agentModels).toEqual([]);
  });

  it("exposes a failed directory without inventing choices, and can retry", async () => {
    let failed = true;
    fakeBackend(() => failed ? { status: 503 } : { status: 200, data: { items: executors } });
    const { result } = mount();
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.agents).toEqual([]);
    expect(result.current.agentModels).toEqual([]);
    failed = false;
    result.current.refetch();
    await waitFor(() => expect(result.current.agentModels).toHaveLength(3));
  });
});

describe("workflow role catalog", () => {
  it("reads the signed-in user's local roles", async () => {
    const { result } = mountAgents();
    await waitFor(() => expect(result.current.data?.length).toBe(2));
    expect(result.current.data?.map((role) => role.name)).toEqual([
      "代码审查",
      "技术写作",
    ]);
  });
});
