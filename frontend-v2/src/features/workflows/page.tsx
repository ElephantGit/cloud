import { useState } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { I18nextProvider } from "react-i18next";
import "@xyflow/react/dist/style.css";
import { Toaster } from "@/components/ui/sonner";
import { createLocalContractsClient } from "./data/contracts-client";
import { ContractsClientProvider } from "./data/contracts-client-provider";
import { LocalWorkflowBackend } from "./data/local-workflow-backend";
import { createWorkflowQueryClient } from "./data/query-client";
import { WorkflowEditor } from "./editor/workflow-editor";
import { workflowI18n } from "./host/i18n";
import { useDarkTheme } from "./host/theme";
import { seedSampleWorkflows } from "./samples/seed";
import { WorkflowSidebar } from "./workflow-sidebar";

/**
 * The workflow page: desktop's library sidebar and definition editor side by side. In desktop the
 * library replaces the app sidebar while the editor owns the workspace pane; here the page holds
 * both, and the local client stands in for the backend until v2-server stores workflows. The page
 * keeps its own query cache with desktop's policy. A browser's first visit finds the sample
 * workflows already in its library.
 */
export default function Page() {
  const [client] = useState(() => {
    const backend = new LocalWorkflowBackend();
    if (backend.startedEmpty) seedSampleWorkflows(backend);
    return createLocalContractsClient(backend);
  });
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
  return (
    <div className="flex min-h-0 min-w-0 flex-1">
      <WorkflowSidebar />
      <WorkflowEditor />
    </div>
  );
}

/**
 * The editor's toasts, themed by frontend-v2's `dark` class rather than a theme provider. Each one
 * has a close button, so a message that covers the canvas controls can be dismissed at once.
 */
function WorkflowToaster() {
  const dark = useDarkTheme();
  return (
    <Toaster
      position="bottom-right"
      theme={dark ? "dark" : "light"}
      closeButton
    />
  );
}
