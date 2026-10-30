import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * Desktop's title-strip drag handle. The browser has no frameless window to drag, so it keeps
 * only the layout: a flexible span whose children cannot take the pointer.
 */
export function DragRegion({
  className,
  children,
}: {
  className?: string;
  children?: ReactNode;
}) {
  return (
    <div
      className={cn("flex min-w-0 flex-1 items-center self-stretch", className)}
    >
      {children !== undefined && (
        <div className="pointer-events-none flex min-w-0 items-center gap-2">
          {children}
        </div>
      )}
    </div>
  );
}

/** Desktop draws its own minimize/maximize/close buttons; the browser window has them already. */
export function WindowControls() {
  return null;
}
