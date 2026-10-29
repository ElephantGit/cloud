import "./test/dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { defaultWorkflowSidebar, useUiStore } from "./host/ui-store";
import Page from "./page";

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <Page />
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

const library = () => screen.getByRole("complementary", { name: "工作流列表" });

/** Creates a workflow through desktop's library menu and name dialog. */
async function createWorkflow(user: ReturnType<typeof userEvent.setup>, name: string) {
  const addMenu = await within(library()).findByRole("button", {
    name: "新建或导入工作流",
  });
  // The library enables its menu once the editor registers its actions.
  await waitFor(() => expect(addMenu).toBeEnabled());
  // Base UI opens the menu on a single click event, as desktop's tests drive it.
  fireEvent.click(addMenu);
  await user.click(await screen.findByRole("menuitem", { name: /新建工作流/ }));
  const dialog = await screen.findByRole("alertdialog", { name: "新建工作流" });
  await user.type(within(dialog).getByLabelText("工作流名称"), name);
  await user.click(within(dialog).getByRole("button", { name: "新建工作流" }));
}

describe("workflow page", () => {
  beforeEach(() => {
    window.localStorage.clear();
    useUiStore.setState({
      sidebarCollapsed: defaultWorkflowSidebar.collapsed,
      sidebarWidth: defaultWorkflowSidebar.width,
    });
  });

  it("opens on the published sample workflows", async () => {
    renderPage();

    for (const name of ["示例：代码审查", "示例：逐段翻译", "循环综合测试：分支与退出"]) {
      expect(await within(library()).findByRole("button", { name })).toBeInTheDocument();
    }
    expect(await screen.findByDisplayValue("示例：代码审查")).toBeInTheDocument();
    expect(await screen.findByText("生效中 · v1")).toBeInTheDocument();
  });

  it("resizes the library from its edge and folds it past the minimum, like the chat history", async () => {
    const user = userEvent.setup();
    renderPage();
    const edge = screen.getByRole("separator", { name: "调整工作流列表宽度" });

    edge.focus();
    await user.keyboard("{ArrowRight}");
    expect(edge).toHaveAttribute("aria-valuenow", "272");
    expect(library()).toHaveStyle({ width: "272px" });

    useUiStore.setState({ sidebarWidth: 200 });
    await user.keyboard("{ArrowLeft}");
    expect(screen.queryByRole("complementary", { name: "工作流列表" })).not.toBeInTheDocument();

    // Folded, the editor header offers only the button that shows the library again.
    expect(screen.queryByRole("button", { name: "返回" })).not.toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "展开侧边栏" }));
    expect(library()).toHaveStyle({ width: "200px" });
  });

  it("lets every toast be dismissed", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByDisplayValue("示例：代码审查");

    await user.click(await screen.findByRole("button", { name: "整理节点" }));
    const toast = await screen.findByText("已整理工作流节点");
    await user.click(screen.getByRole("button", { name: "Close toast" }));
    await waitFor(() => expect(toast).not.toBeInTheDocument());
  });

  it("creates a workflow from the library and opens its canvas at the Start node", async () => {
    const user = userEvent.setup();
    renderPage();

    await createWorkflow(user, "审查流程");

    expect(
      await within(library()).findByRole("button", { name: "审查流程" }),
    ).toBeInTheDocument();
    expect(await screen.findByDisplayValue("审查流程")).toBeInTheDocument();
    const canvas = await screen.findByLabelText("工作流画布");
    expect((await within(canvas).findAllByText("开始")).length).toBeGreaterThan(0);
  });

  it("keeps workflows across a reload of the page", async () => {
    const user = userEvent.setup();
    const first = renderPage();
    await createWorkflow(user, "留存");
    await within(library()).findByRole("button", { name: "留存" });
    first.unmount();

    renderPage();
    expect(
      await within(library()).findByRole("button", { name: "留存" }),
    ).toBeInTheDocument();
  });
});
