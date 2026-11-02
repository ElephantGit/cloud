import { useMemo } from "react";
import type { PluginLogo } from "../contracts";
import { useWorkflowExecutors } from "./catalog-hooks";

/**
 * Describes one agent this installation can bind a session to.
 *
 * Every field comes from the package that supplies the agent. Nothing about which agents exist,
 * what they are called, or how they are drawn is known at build time: an agent arrives with an
 * installed plugin and leaves when that plugin is uninstalled.
 */
export interface AgentEntry {
  /**
   * The agent's persisted, namespaced identity — the package's whole `namespace/name` id, which
   * is exactly what a session stores as its `agentRef` and what the backend keys its runtime by.
   */
  agentRef: string;
  /** Plugin title shown in the pickers. */
  label: string;
  /** Host-local asset URLs for the package's brand mark, absent when it ships none. */
  logo: PluginLogo | null;
}

/**
 * Lists runnable agents from Chat's execution directory, preserving the backend's order.
 */
export function useAgentCatalog(): AgentEntry[] {
  const { data } = useWorkflowExecutors();
  return useMemo(
    () =>
      [...new Map((data?.items ?? []).map((executor) => [executor.agentId, {
        agentRef: executor.agentId,
        label: executor.agentName,
        logo: null,
      }])).values()],
    [data],
  );
}

/**
 * Resolves how one bound agent should be named and drawn.
 *
 * A session can outlive the package that supplied its agent, so a miss is an ordinary state
 * rather than an error. Callers render the raw identity in that case, which is the only honest
 * thing left to show.
 */
export function useAgentEntry(agentRef: string | null): AgentEntry | undefined {
  const catalog = useAgentCatalog();
  return catalog.find((entry) => entry.agentRef === agentRef);
}

/** Names one agent for display, falling back to its raw identity when nothing supplies it. */
export function agentLabel(
  catalog: readonly AgentEntry[],
  agentRef: string,
): string {
  return (
    catalog.find((entry) => entry.agentRef === agentRef)?.label ?? agentRef
  );
}
