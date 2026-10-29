/**
 * Sample workflows written for the v2 page, in desktop's export format. Together with desktop's
 * three Loop examples beside this file they cover all eight executable node kinds: the review
 * chain is the demo's baseline path (Start → Agent → Condition → Aggregator → Output), and the
 * translation batch shows an Iteration.
 */

const executor = { agentCli: "ora/claude-code", modelId: "claude-sonnet-5-5" };

function agentConfig(prompt: string) {
  return {
    schemaVersion: 3,
    executor,
    roleId: "",
    skills: [],
    mcps: [],
    prompt,
    interactive: false,
  };
}

/** Start → Agent → Condition → (two Agents) → Aggregator → Output. */
export const reviewChainSample = {
  id: "sample-code-review",
  name: "示例：代码审查",
  description:
    "按仓库和分支审查改动；发现阻塞问题时整理修复清单，否则写通过说明，聚合后输出。",
  viewport: { x: 40, y: 120, zoom: 0.7 },
  nodes: [
    {
      id: "start",
      type: "workflow",
      position: { x: 0, y: 160 },
      deletable: false,
      data: {
        kind: "start",
        title: "开始：审查目标",
        description: "",
        inputVariables: [
          {
            name: "repository",
            displayName: "仓库",
            fieldType: "text-input",
            valueType: "string",
            required: true,
            value: "ora-space/cloud",
          },
          {
            name: "branch",
            displayName: "分支",
            fieldType: "text-input",
            valueType: "string",
            required: true,
            value: "main",
          },
        ],
      },
    },
    {
      id: "review",
      type: "workflow",
      position: { x: 340, y: 120 },
      data: {
        kind: "agent",
        title: "Agent：审查改动",
        description: "",
        agentConfig: agentConfig(
          "审查 {{#start.repository#}} 的 {{#start.branch#}} 分支相对 main 的改动。\n" +
            "存在必须修复才能合并的问题时，第一行只写 BLOCK；否则第一行只写 PASS。\n" +
            "从第二行起逐条列出发现，写明文件和行号。",
        ),
      },
    },
    {
      id: "verdict",
      type: "workflow",
      position: { x: 700, y: 140 },
      data: {
        kind: "condition",
        title: "条件分支：是否阻塞",
        description: "",
        cases: [
          {
            id: "blocked",
            logic: "and",
            conditions: [
              {
                variableSelector: ["review", "output"],
                operator: "contains",
                value: "BLOCK",
              },
            ],
          },
        ],
      },
    },
    {
      id: "fix-list",
      type: "workflow",
      position: { x: 1060, y: 20 },
      data: {
        kind: "agent",
        title: "Agent：整理修复清单",
        description: "",
        agentConfig: agentConfig(
          "根据审查结果整理一份按严重程度排序的修复清单：\n{{#review.output#}}",
        ),
      },
    },
    {
      id: "approval",
      type: "workflow",
      position: { x: 1060, y: 300 },
      data: {
        kind: "agent",
        title: "Agent：写通过说明",
        description: "",
        agentConfig: agentConfig(
          "根据审查结果写一段简短的通过说明，列出值得注意的非阻塞建议：\n{{#review.output#}}",
        ),
      },
    },
    {
      id: "merge",
      type: "workflow",
      position: { x: 1420, y: 160 },
      data: {
        kind: "aggregator",
        title: "变量聚合器：审查结论",
        description: "",
        aggregatorConfig: {
          variables: [
            ["fix-list", "output"],
            ["approval", "output"],
          ],
        },
      },
    },
    {
      id: "output",
      type: "workflow",
      position: { x: 1760, y: 160 },
      data: {
        kind: "output",
        title: "输出",
        description: "",
        outputs: [
          { name: "verdict", variableSelector: ["review", "output"] },
          { name: "report", variableSelector: ["merge", "output"] },
        ],
      },
    },
  ],
  edges: [
    { id: "e-start-review", type: "workflow", source: "start", target: "review" },
    { id: "e-review-verdict", type: "workflow", source: "review", target: "verdict" },
    {
      id: "e-verdict-fix",
      type: "workflow",
      source: "verdict",
      target: "fix-list",
      sourceHandle: "blocked",
    },
    {
      id: "e-verdict-approval",
      type: "workflow",
      source: "verdict",
      target: "approval",
      sourceHandle: "else",
    },
    { id: "e-fix-merge", type: "workflow", source: "fix-list", target: "merge" },
    { id: "e-approval-merge", type: "workflow", source: "approval", target: "merge" },
    { id: "e-merge-output", type: "workflow", source: "merge", target: "output" },
  ],
  annotations: [],
  globalVariables: [],
};

/** Start → Iteration (an Agent per paragraph) → Output. */
export const translationBatchSample = {
  id: "sample-translation-batch",
  name: "示例：逐段翻译",
  description: "把输入的段落列表逐段译成英文，失败的段落跳过，最后收集成一个数组输出。",
  viewport: { x: 60, y: 140, zoom: 0.75 },
  nodes: [
    {
      id: "start",
      type: "workflow",
      position: { x: 0, y: 120 },
      deletable: false,
      data: {
        kind: "start",
        title: "开始：待译段落",
        description: "",
        inputVariables: [
          {
            name: "paragraphs",
            displayName: "段落列表",
            fieldType: "json",
            valueType: "array[string]",
            required: true,
            value: '["工作流把多个 Agent 串成一条可重复的流程。", "每个节点的输出都能被后续节点引用。"]',
          },
        ],
      },
    },
    {
      id: "per-paragraph",
      type: "workflow",
      position: { x: 340, y: 40 },
      initialWidth: 560,
      initialHeight: 340,
      data: {
        kind: "iteration",
        title: "迭代：每个段落",
        description: "",
        iterationConfig: {
          iteratorSelector: ["start", "paragraphs"],
          collectSelector: ["translate", "output"],
          errorStrategy: "continue",
          maxIterations: 50,
        },
      },
    },
    {
      id: "translate",
      type: "workflow",
      position: { x: 160, y: 120 },
      parentId: "per-paragraph",
      data: {
        kind: "agent",
        title: "Agent：翻译",
        description: "",
        agentConfig: agentConfig(
          "把下面这段中文译成自然的英文，只输出译文：\n{{#per-paragraph.item#}}",
        ),
      },
    },
    {
      id: "output",
      type: "workflow",
      position: { x: 1020, y: 120 },
      data: {
        kind: "output",
        title: "输出",
        description: "",
        outputs: [
          { name: "translations", variableSelector: ["per-paragraph", "output"] },
        ],
      },
    },
  ],
  edges: [
    {
      id: "e-start-iteration",
      type: "workflow",
      source: "start",
      target: "per-paragraph",
    },
    {
      id: "e-iteration-entry",
      type: "workflow",
      source: "per-paragraph",
      target: "translate",
      sourceHandle: "iteration-entry",
    },
    {
      id: "e-iteration-output",
      type: "workflow",
      source: "per-paragraph",
      target: "output",
    },
  ],
  annotations: [],
  globalVariables: [],
};
