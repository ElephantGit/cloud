import { useSyncExternalStore } from "react";

/** Whether the page renders dark: frontend-v2 marks dark mode with the `dark` class on `<html>`. */
export function useDarkTheme(): boolean {
  return useSyncExternalStore(subscribe, () =>
    document.documentElement.classList.contains("dark"),
  );
}

function subscribe(onChange: () => void): () => void {
  const observer = new MutationObserver(onChange);
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["class"],
  });
  return () => observer.disconnect();
}
