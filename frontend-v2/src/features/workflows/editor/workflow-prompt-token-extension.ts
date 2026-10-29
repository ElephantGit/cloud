import { ReactNodeViewRenderer } from "@tiptap/react";
import { PromptToken } from "../composer-core/composer";
import { PromptTokenNodeView } from "./workflow-prompt-variable-token";

/** PromptToken variant used by the workflow prompt editor, rendered via React. */
export const WorkflowPromptVariableToken = PromptToken.extend({
  addNodeView() {
    return ReactNodeViewRenderer(PromptTokenNodeView);
  },
});
