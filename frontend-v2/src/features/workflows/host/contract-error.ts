import type { TFunction } from "i18next";
import { ContractError } from "../contracts";
import { workflowTranslationResources } from "./i18n";

/** Localizes a refused local operation by its code, as desktop does for remote contract errors. */
export function localizeContractError(error: unknown, t: TFunction): string {
  if (error instanceof ContractError) {
    const key = `errors.${error.code}`;
    if (key in workflowTranslationResources["zh-CN"]) {
      return t(key, error.params);
    }
  }
  return t("errors.unknown");
}
