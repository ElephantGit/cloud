import { describe, expect, it } from "vitest";
import {
  LocalWorkflowBackend,
  type WorkflowBackendStorage,
} from "../data/local-workflow-backend";
import { seedSampleWorkflows } from "./seed";

function memoryStorage(): WorkflowBackendStorage {
  let value: string | null = null;
  return {
    load: () => value,
    save: (serialized) => {
      value = serialized;
    },
  };
}

describe("seedSampleWorkflows", () => {
  it("publishes every sample, the authored ones first", () => {
    const backend = new LocalWorkflowBackend({ storage: memoryStorage() });
    expect(backend.startedEmpty).toBe(true);

    seedSampleWorkflows(backend);

    const { workflows } = backend.list();
    expect(workflows.map((workflow) => [workflow.name, workflow.publishedVersion])).toEqual([
      ["示例：代码审查", "v1"],
      ["示例：逐段翻译", "v1"],
      ["循环综合测试：分支与退出", "v1"],
      ["循环综合测试：并行退出取消", "v1"],
      ["最大循环轮次开关测试（无需模型）", "v1"],
    ]);
  });

  it("covers all eight executable node kinds with no spare nodes", () => {
    const backend = new LocalWorkflowBackend({ storage: memoryStorage() });
    seedSampleWorkflows(backend);

    const kinds = new Set<string>();
    for (const { id } of backend.list().workflows) {
      const { draft } = backend.get({ workflowId: id });
      const graph = JSON.parse(draft.graph) as { nodes: { data: { kind: string } }[] };
      for (const node of graph.nodes) kinds.add(node.data.kind);
      expect(backend.analyze({ graph: draft.graph }), id).toEqual({
        unusedNodeIds: [],
        unrecognizedNodes: [],
      });
    }
    expect([...kinds].sort()).toEqual(
      ["agent", "aggregator", "condition", "iteration", "loop", "loopExit", "output", "start"].sort(),
    );
  });

  it("points desktop's Loop examples at the demo executor", () => {
    const backend = new LocalWorkflowBackend({ storage: memoryStorage() });
    seedSampleWorkflows(backend);
    const loop = backend.list().workflows.find((workflow) =>
      workflow.name.startsWith("循环综合测试：分支"),
    )!;
    const graph = JSON.parse(backend.get({ workflowId: loop.id }).draft.graph) as {
      nodes: { data: { agentConfig?: { executor: unknown; roleId: string } } }[];
    };
    const agents = graph.nodes.flatMap((node) =>
      node.data.agentConfig === undefined ? [] : [node.data.agentConfig],
    );
    expect(agents.length).toBeGreaterThan(0);
    for (const agent of agents) {
      expect(agent).toMatchObject({
        executor: { agentCli: "ora/claude-code", modelId: "claude-sonnet-5-5" },
        roleId: "",
      });
    }
  });
});
