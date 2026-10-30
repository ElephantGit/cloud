import { createInstance } from "i18next";
import { initReactI18next } from "react-i18next";
import { workflowEditorTranslations } from "../editor/translations";
import { workflowNodeTranslations } from "../node-chrome/translations";

/**
 * The app-wide strings the copied desktop code reads beside its own bundles, in desktop's wording.
 * The page only renders zh-CN; en-US is kept so desktop's component tests run unchanged.
 */
const hostTranslations = {
  "zh-CN": {
    "chat.copy": "复制",
    "chat.cut": "剪切",
    "chat.modelSelector.agent": "Agent",
    "chat.modelSelector.loading": "加载中…",
    "chat.modelSelector.model": "模型",
    "chat.paste": "粘贴",
    "chat.selectAll": "全选",
    "common.cancel": "取消",
    "common.delete": "删除",
    "common.retry": "重试",
    "common.save": "保存",
    "errors.unknown": "发生未知错误。",
    "errors.workflow_run_graph_parse": "工作流图解析失败。",
    "settings.roles.loading": "正在加载角色…",
    "settings.skills.loading": "正在加载技能…",
    "sidebar.back": "返回",
    "sidebar.clearSearch": "清除搜索",
    "sidebar.collapse": "收起侧边栏",
    "sidebar.expand": "展开侧边栏",
    "sidebar.workflows": "工作流",
  },
  "en-US": {
    "chat.copy": "Copy",
    "chat.cut": "Cut",
    "chat.modelSelector.agent": "Agent",
    "chat.modelSelector.loading": "Loading…",
    "chat.modelSelector.model": "Model",
    "chat.paste": "Paste",
    "chat.selectAll": "Select All",
    "common.cancel": "Cancel",
    "common.delete": "Delete",
    "common.retry": "Retry",
    "common.save": "Save",
    "errors.unknown": "An unknown error occurred.",
    "errors.workflow_run_graph_parse": "Failed to parse the workflow graph.",
    "settings.roles.loading": "Loading roles...",
    "settings.skills.loading": "Loading skills...",
    "sidebar.back": "Back",
    "sidebar.clearSearch": "Clear search",
    "sidebar.collapse": "Collapse sidebar",
    "sidebar.expand": "Expand sidebar",
    "sidebar.workflows": "Workflows",
  },
};

export type WorkflowLocale = "zh-CN" | "en-US";

/** Every string the workflow page renders, per locale. */
export const workflowTranslationResources = {
  "zh-CN": {
    ...hostTranslations["zh-CN"],
    ...workflowNodeTranslations["zh-CN"],
    ...workflowEditorTranslations["zh-CN"],
  },
  "en-US": {
    ...hostTranslations["en-US"],
    ...workflowNodeTranslations["en-US"],
    ...workflowEditorTranslations["en-US"],
  },
};

/**
 * Builds an isolated i18next instance over the workflow bundles, as desktop's `appI18n` is.
 * `extra` adds a page's own bundle, such as the run view's.
 */
export function createWorkflowI18n(
  locales: readonly WorkflowLocale[],
  extra: Partial<Record<WorkflowLocale, Record<string, string>>> = {},
) {
  const instance = createInstance();
  void instance.use(initReactI18next).init({
    resources: Object.fromEntries(
      locales.map((locale) => [
        locale,
        {
          translation: {
            ...workflowTranslationResources[locale],
            ...extra[locale],
          },
        },
      ]),
    ),
    lng: "zh-CN",
    fallbackLng: "zh-CN",
    keySeparator: false,
    interpolation: { escapeValue: false },
    initAsync: false,
  });
  return instance;
}

/** The page's instance: frontend-v2 is Chinese only. */
export const workflowI18n = createWorkflowI18n(["zh-CN"]);
