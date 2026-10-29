import { create } from "zustand";

/**
 * The slice of desktop's app UI store the editor reads. In desktop the workflow library sits in
 * the app sidebar and the editor replaces the workspace; here the page owns both, so "leave the
 * editor" returns to the library and the settings shortcuts only explain where they would go.
 */
interface WorkflowUiState {
  sidebarCollapsed: boolean;
  setSidebarCollapsed: (collapsed: boolean) => void;
  setWorkflowEditorOpen: (open: boolean) => void;
  openSettingsAt: (category: string) => void;
  openPluginSettings: (request: { kind: string; [key: string]: unknown }) => void;
}

export const useUiStore = create<WorkflowUiState>()((set) => ({
  sidebarCollapsed: false,
  setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),
  setWorkflowEditorOpen: (open) => {
    if (!open) set({ sidebarCollapsed: false });
  },
  openSettingsAt: () => {},
  openPluginSettings: () => {},
}));
