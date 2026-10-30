import { IconLoader2 } from "@tabler/icons-react";
import { cn } from "@/lib/utils";

/** Desktop's `@ora/ui` spinner: a turning loader that screen readers announce as loading. */
export function Spinner({ className }: { className?: string }) {
  return (
    <IconLoader2
      role="status"
      aria-label="加载中"
      className={cn("size-4 animate-spin", className)}
    />
  );
}
