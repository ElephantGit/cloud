import type { ComponentProps } from "react";
import type { PanelImperativeHandle } from "react-resizable-panels";
import {
  ResizableHandle,
  ResizablePanel as GeneratedResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable";
import { cn } from "@/lib/utils";

export type ResizablePanelHandle = PanelImperativeHandle;

/**
 * Desktop's panel: the primitive gives the panel body no definite height, so a child sized with
 * `h-full` grows to its content and the pane scrolls past the window. A clipped flex column lets
 * the editor's regions size with `flex-1` + `min-h-0` and scroll only where they opt in.
 */
function ResizablePanel({
  className,
  style,
  ...props
}: ComponentProps<typeof GeneratedResizablePanel>) {
  return (
    <GeneratedResizablePanel
      className={cn("flex min-h-0 min-w-0 flex-col", className)}
      style={{ overflow: "hidden", ...style }}
      {...props}
    />
  );
}

export { ResizableHandle, ResizablePanel, ResizablePanelGroup };
