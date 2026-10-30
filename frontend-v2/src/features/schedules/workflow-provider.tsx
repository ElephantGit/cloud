import { useState, type ReactNode } from 'react'
import { ContractsClientProvider } from '@/lib/workflows/data/contracts-client-provider'
import { createLocalContractsClient } from '@/lib/workflows/data/contracts-client'
import { LocalWorkflowBackend } from '@/lib/workflows/data/local-workflow-backend'
import { seedSampleWorkflows } from '@/lib/workflows/samples/seed'

/** Uses the editor's browser library until the shared workflow backend is connected. */
export function ScheduleWorkflowProvider({ children }: { children: ReactNode }) {
  const [client] = useState(() => {
    const backend = new LocalWorkflowBackend()
    seedSampleWorkflows(backend)
    return createLocalContractsClient(backend)
  })
  return <ContractsClientProvider client={client}>{children}</ContractsClientProvider>
}
