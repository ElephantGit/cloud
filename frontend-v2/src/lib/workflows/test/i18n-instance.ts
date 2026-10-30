import {
  createWorkflowI18n,
  workflowTranslationResources,
} from "../host/i18n";

// Desktop's `i18n/i18n-instance` for the copied component tests, which switch to en-US and look up
// resources by locale.
export const appI18n = createWorkflowI18n(["zh-CN", "en-US"]);
export const translationResources = workflowTranslationResources;
export type Locale = keyof typeof workflowTranslationResources;
