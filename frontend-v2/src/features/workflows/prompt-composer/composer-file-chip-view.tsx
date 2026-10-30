import { IconFile, IconFolder } from "@tabler/icons-react";
import { NodeViewWrapper, type NodeViewProps } from "@tiptap/react";
import {
  composerFileAttrsFromUnknown,
  composerFileChipTitle,
  composerFileLabel,
} from "../composer-core/composer";

/**
 * Desktop's path mention chip without its jump to the Files/Changes pane, which the browser page
 * does not have. Workflow prompts mention variables rather than files; a pasted mention still
 * shows its icon and basename.
 */
export function ComposerFileChipView({ node }: NodeViewProps) {
  const attrs = composerFileAttrsFromUnknown(node.attrs);
  const Icon = attrs.kind === "directory" ? IconFolder : IconFile;
  return (
    <NodeViewWrapper
      as="span"
      className="inline-flex items-center gap-1 align-baseline"
      title={composerFileChipTitle(attrs)}
    >
      <Icon className="size-3.5 shrink-0 text-muted-foreground" />
      <span>{composerFileLabel(attrs)}</span>
    </NodeViewWrapper>
  );
}
