import { QueryClient } from "@tanstack/react-query";

/**
 * Desktop's app-shell cache policy, which the copied editor's cache patches and autosave rely on.
 * With frontend-v2's default `staleTime: 0`, a freshly seeded draft would refetch on mount and
 * could overwrite the publish that follows an import with the pre-publish answer.
 */
export function createWorkflowQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: 1,
        refetchOnWindowFocus: false,
        staleTime: 30_000,
      },
      mutations: {
        retry: 0,
      },
    },
  });
}
