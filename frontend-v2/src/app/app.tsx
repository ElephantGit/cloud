import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, type createBrowserRouter } from 'react-router'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ThemeProvider } from 'next-themes'

const queryClient = new QueryClient()

/** Providers shared by every page; the router decides which feature renders. */
export function App({
  router,
}: {
  router: ReturnType<typeof createBrowserRouter>
}) {
  return (
    <QueryClientProvider client={queryClient}>
      {/* createRoot cannot execute the library's SSR bootstrap. main.tsx initializes before mount. */}
      <ThemeProvider
        attribute="class"
        defaultTheme="light"
        enableSystem
        storageKey="ora-v2-theme"
        disableTransitionOnChange
        scriptProps={{ type: 'text/plain' }}
      >
        <TooltipProvider>
          <RouterProvider router={router} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}
