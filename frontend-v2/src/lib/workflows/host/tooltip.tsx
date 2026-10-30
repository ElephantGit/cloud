import type { ComponentProps } from "react";
import { TooltipContent as GeneratedTooltipContent } from "@/components/ui/tooltip";

/** Desktop's tooltip popup announces itself as a tooltip; the generated one carries no role. */
export function TooltipContent(
  props: ComponentProps<typeof GeneratedTooltipContent>,
) {
  return <GeneratedTooltipContent role="tooltip" {...props} />;
}
