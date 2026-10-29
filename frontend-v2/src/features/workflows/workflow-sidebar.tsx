import type { KeyboardEvent, PointerEvent } from "react";
import { PanelLeftCloseIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { WorkflowEditorList } from "./editor/workflow-editor-list";
import { useUiStore } from "./host/ui-store";

const minWidth = 200;
const maxWidth = 480;
/** Dragging the edge this close to the icon rail folds the column away. */
const foldBelow = 120;

/**
 * Desktop's workflow library in a column that resizes and folds like the chat history: drag its
 * right edge (or use the arrow keys on it) to resize, past the rail to fold it away, and Enter to
 * toggle. Folded, the editor header shows the button that brings it back.
 */
export function WorkflowSidebar() {
  const collapsed = useUiStore((state) => state.sidebarCollapsed);
  const width = useUiStore((state) => state.sidebarWidth);
  const setSidebarCollapsed = useUiStore((state) => state.setSidebarCollapsed);
  return (
    <>
      {!collapsed && (
        <aside
          aria-label="工作流列表"
          style={{ width }}
          className="flex shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground"
        >
          <div className="flex h-12 shrink-0 items-center justify-between pr-2 pl-4">
            <h1 className="text-sm font-semibold">工作流</h1>
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label="收起工作流列表"
              onClick={() => setSidebarCollapsed(true)}
            >
              <PanelLeftCloseIcon />
            </Button>
          </div>
          <WorkflowEditorList />
        </aside>
      )}
      <ResizeHandle />
    </>
  );
}

/** The column's right edge, as in the chat history. */
function ResizeHandle() {
  const collapsed = useUiStore((state) => state.sidebarCollapsed);
  const width = useUiStore((state) => state.sidebarWidth);
  const { setSidebarCollapsed, setSidebarWidth } = useUiStore.getState();
  const resizeTo = (next: number) => {
    if (next < foldBelow) setSidebarCollapsed(true);
    else setSidebarWidth(Math.min(maxWidth, Math.max(minWidth, next)));
  };

  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    event.preventDefault();
    const handle = event.currentTarget;
    const startX = event.clientX;
    const start = collapsed ? 0 : width;
    handle.setPointerCapture(event.pointerId);
    document.body.style.cursor = "col-resize";
    const move = (moveEvent: globalThis.PointerEvent) =>
      resizeTo(start + moveEvent.clientX - startX);
    const stop = () => {
      document.body.style.cursor = "";
      handle.removeEventListener("pointermove", move);
      handle.removeEventListener("pointerup", stop);
      handle.removeEventListener("pointercancel", stop);
    };
    handle.addEventListener("pointermove", move);
    handle.addEventListener("pointerup", stop);
    handle.addEventListener("pointercancel", stop);
  };
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const step = { ArrowLeft: -16, ArrowRight: 16 }[event.key];
    if (step !== undefined) {
      resizeTo(
        collapsed
          ? minWidth
          : width === minWidth && step < 0
            ? 0
            : width + step,
      );
    } else if (event.key === "Enter") {
      setSidebarCollapsed(!collapsed);
    } else {
      return;
    }
    event.preventDefault();
  };

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label="调整工作流列表宽度"
      aria-valuemin={0}
      aria-valuemax={maxWidth}
      aria-valuenow={collapsed ? 0 : width}
      tabIndex={0}
      onPointerDown={onPointerDown}
      onKeyDown={onKeyDown}
      className="relative z-10 -mx-1 w-2 shrink-0 cursor-col-resize touch-none outline-none after:absolute after:inset-y-0 after:left-1/2 after:w-0.5 after:-translate-x-1/2 after:transition-colors hover:after:bg-primary/40 focus-visible:after:bg-primary active:after:bg-primary/60"
    />
  );
}
