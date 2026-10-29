import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, type createBrowserRouter } from 'react-router'
import { TooltipProvider } from '@/components/ui/tooltip'

const queryClient = new QueryClient()

/** Providers shared by every page; the router decides which feature renders. */
export function App({ router }: { router: ReturnType<typeof createBrowserRouter> }) {
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  )
}
