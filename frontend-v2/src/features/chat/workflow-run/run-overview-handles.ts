import { resolveConditionCases } from "@/lib/workflows/mock";
import type { WorkflowNodeData } from "@/lib/workflows/runtime";

/** Returns every persisted branch port a Condition exposes, including its fallback edge. */
export function resolveRunOverviewSourceHandleIds(
  data: WorkflowNodeData,
): string[] | null {
  if (data.kind !== "condition") return null;
  return [
    ...resolveConditionCases(data).map((conditionCase) => conditionCase.id),
    "else",
  ];
}
