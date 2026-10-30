import "./dom";
import type { ReactNode } from "react";
import { I18nextProvider } from "react-i18next";
import { appI18n } from "./i18n-instance";

/** Desktop's `AppI18nProvider` over the test instance. */
export function AppI18nProvider({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={appI18n}>{children}</I18nextProvider>;
}
