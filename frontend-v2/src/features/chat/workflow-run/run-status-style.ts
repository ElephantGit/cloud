import type {
  GraphWorkflowNodeStatus,
  GraphWorkflowRunStatus,
} from "@/lib/workflows/runtime";
import { workflowRunTranslations } from "./translations";

/**
 * Node is actively executing or blocked on HITL—the only spinner-worthy states. A node waiting to
 * retry is deliberately excluded: it has no session and nothing is executing until the wait ends.
 */
export function isNodeWorking(
  status: GraphWorkflowRunStatus | GraphWorkflowNodeStatus,
): boolean {
  return status === "running" || status === "awaiting_input";
}

/** Shared run/node status chrome —color is never the only signal (dot + label + tone). */
export function runStatusTone(
  status: GraphWorkflowRunStatus | GraphWorkflowNodeStatus,
): {
  dot: string;
  ring: string;
  badge: string;
  labelKey: string;
} {
  switch (status) {
    case "running":
      return {
        dot: "bg-info",
        ring: "border-info/50 ring-info/20",
        badge: "border-info/30 bg-info/10 text-info",
        labelKey: "workflowRun.status.running",
      };
    case "awaiting_input":
      // Amber matches HITL “must handle—chrome on the dock and stage card.
      return {
        dot: "bg-warning",
        ring: "border-warning/50 ring-warning/20",
        badge:
          "border-warning/35 bg-warning/10 text-warning",
        labelKey: "workflowRun.status.awaiting_input",
      };
    case "retry_waiting":
      // Orange sits between amber (“needs you”) and rose (failed): an attempt failed and the
      // engine will run the node again by itself, so no action is expected from the user.
      return {
        dot: "bg-caution",
        ring: "border-caution/45 ring-caution/15",
        badge:
          "border-caution/30 bg-caution/10 text-caution",
        labelKey: "workflowRun.status.retry_waiting",
      };
    case "succeeded":
      return {
        dot: "bg-success",
        ring: "border-success/45 ring-success/15",
        badge:
          "border-success/30 bg-success/10 text-success",
        labelKey: "workflowRun.status.succeeded",
      };
    case "failed":
      return {
        dot: "bg-destructive",
        ring: "border-destructive/45 ring-destructive/15",
        badge:
          "border-destructive/30 bg-destructive/10 text-destructive",
        labelKey: "workflowRun.status.failed",
      };
    case "cancelled":
      return {
        dot: "bg-muted-foreground/50",
        ring: "border-muted-foreground/40 ring-muted-foreground/10",
        badge:
          "border-muted-foreground/30 bg-muted text-muted-foreground",
        labelKey: "workflowRun.status.cancelled",
      };
    case "pending":
      return {
        dot: "bg-warning/60",
        ring: "border-warning/40 ring-warning/10",
        badge:
          "border-warning/30 bg-warning/10 text-warning",
        labelKey: "workflowRun.status.pending",
      };
    case "idle":
      return {
        dot: "bg-muted-foreground/35",
        ring: "border-border ring-transparent",
        badge: "border-border bg-muted/60 text-muted-foreground",
        labelKey: "workflowRun.nodeStatus.idle",
      };
    case "inactive":
      // A node behind a lost condition branch: never ran, never will. Dimmed so the active path
      // reads clearly without implying failure.
      return {
        dot: "bg-muted-foreground/20",
        ring: "border-transparent ring-transparent",
        badge: "border-transparent bg-muted/30 text-muted-foreground/60",
        labelKey: "workflowRun.nodeStatus.inactive",
      };
  }
}

/** A status's Chinese label, for chat's own components that render outside the run i18n scope. */
export function runStatusLabel(
  status: GraphWorkflowRunStatus | GraphWorkflowNodeStatus,
): string {
  const key = runStatusTone(status)
    .labelKey as keyof (typeof workflowRunTranslations)["zh-CN"];
  return workflowRunTranslations["zh-CN"][key];
}
