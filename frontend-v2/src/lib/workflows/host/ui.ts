// Stands in for desktop's `@ora/ui`: both are shadcn base-nova, so the editor's imports resolve to
// frontend-v2's generated components. Only what desktop customized for the editor is added here.
export { cn } from "@/lib/utils";
export * from "@/components/ui/alert-dialog";
export * from "@/components/ui/badge";
export * from "@/components/ui/button";
export * from "@/components/ui/checkbox";
export * from "@/components/ui/command";
export * from "@/components/ui/context-menu";
export * from "@/components/ui/dialog";
export * from "@/components/ui/dropdown-menu";
export * from "@/components/ui/input";
export * from "@/components/ui/native-select";
export * from "@/components/ui/label";
export * from "@/components/ui/popover";
export * from "@/components/ui/select";
export * from "@/components/ui/switch";
export * from "@/components/ui/textarea";
export {
  Tooltip,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
export { TooltipContent } from "./tooltip";
export { Toaster } from "@/components/ui/sonner";
export { toast } from "sonner";
export {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
  type ResizablePanelHandle,
} from "./resizable";
