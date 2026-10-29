import { useState } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { I18nextProvider } from "react-i18next";
import { IconLayoutSidebarLeftCollapse } from "@tabler/icons-react";
import "@xyflow/react/dist/style.css";
import { Button } from "@/components/ui/button";
import { Toaster } from "@/components/ui/sonner";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { createLocalContractsClient } from "./data/contracts-client";
import { ContractsClientProvider } from "./data/contracts-client-provider";
import { createWorkflowQueryClient } from "./data/query-client";
import { WorkflowEditor } from "./editor/workflow-editor";
import { WorkflowEditorList } from "./editor/workflow-editor-list";
import { workflowI18n } from "./host/i18n";
import { useDarkTheme } from "./host/theme";
import { useUiStore } from "./host/ui-store";

/**
 * The workflow page: desktop's library sidebar and definition editor side by side. In desktop the
 * library replaces the app sidebar while the editor owns the workspace pane; here the page holds
 * both, and the local client stands in for the backend until v2-server stores workflows. The page
 * keeps its own query cache with desktop's policy.
 */
export default function Page() {
  const [client] = useState(() => createLocalContractsClient());
  const [queryClient] = useState(createWorkflowQueryClient);
  return (
    <I18nextProvider i18n={workflowI18n}>
      <QueryClientProvider client={queryClient}>
        <ContractsClientProvider client={client}>
          <WorkflowWorkspace />
          <WorkflowToaster />
        </ContractsClientProvider>
      </QueryClientProvider>
    </I18nextProvider>
  );
}

function WorkflowWorkspace() {
  const sidebarCollapsed = useUiStore((state) => state.sidebarCollapsed);
  const setSidebarCollapsed = useUiStore((state) => state.setSidebarCollapsed);
  return (
    <div className="flex min-h-0 min-w-0 flex-1">
      {!sidebarCollapsed && (
        <aside className="flex w-64 shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground">
          <header className="flex h-14 shrink-0 items-center gap-2 px-3">
            <h1 className="flex-1 text-[15px] font-semibold tracking-[-0.01em]">
              工作流
            </h1>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => setSidebarCollapsed(true)}
                    aria-label="收起侧边栏"
                  />
                }
              >
                <IconLayoutSidebarLeftCollapse />
              </TooltipTrigger>
              <TooltipContent>收起侧边栏</TooltipContent>
            </Tooltip>
          </header>
          <WorkflowEditorList />
        </aside>
      )}
      <WorkflowEditor />
    </div>
  );
}

/** The editor's toasts, themed by frontend-v2's `dark` class rather than a theme provider. */
function WorkflowToaster() {
  const dark = useDarkTheme();
  return <Toaster position="bottom-right" theme={dark ? "dark" : "light"} />;
}
