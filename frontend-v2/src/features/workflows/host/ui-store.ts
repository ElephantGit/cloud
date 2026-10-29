import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

/** The library column's width, and whether it is folded away. */
export interface WorkflowSidebarLayout {
  width: number;
  collapsed: boolean;
}

export const defaultWorkflowSidebar: WorkflowSidebarLayout = {
  width: 256,
  collapsed: false,
};

/**
 * The slice of desktop's app UI store the editor reads, plus the library column's layout. In
 * desktop the workflow library sits in the app sidebar and the editor replaces the workspace;
 * here the page owns both, so "leave the editor" returns to the library and the settings
 * shortcuts have nowhere to go. The layout persists in the browser, like the chat column's.
 */
interface WorkflowUiState {
  sidebarCollapsed: boolean;
  sidebarWidth: number;
  setSidebarCollapsed: (collapsed: boolean) => void;
  setSidebarWidth: (width: number) => void;
  setWorkflowEditorOpen: (open: boolean) => void;
  openSettingsAt: (category: string) => void;
  openPluginSettings: (request: { kind: string; [key: string]: unknown }) => void;
}

export const useUiStore = create<WorkflowUiState>()(
  persist(
    (set) => ({
      sidebarCollapsed: defaultWorkflowSidebar.collapsed,
      sidebarWidth: defaultWorkflowSidebar.width,
      setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),
      setSidebarWidth: (width) =>
        set({ sidebarWidth: width, sidebarCollapsed: false }),
      setWorkflowEditorOpen: (open) => {
        if (!open) set({ sidebarCollapsed: false });
      },
      openSettingsAt: () => {},
      openPluginSettings: () => {},
    }),
    {
      name: "ora-v2-workflows-layout",
      version: 1,
      storage: createJSONStorage(() => localStorage),
      partialize: (state) => ({
        sidebarCollapsed: state.sidebarCollapsed,
        sidebarWidth: state.sidebarWidth,
      }),
    },
  ),
);
