import { Button } from '@/components/ui/button'
import { errorMessage } from './shared'

export function ErrorNotice({ error, retry }: { error: unknown; retry?: () => void }) {
  return <div role="alert" className="rounded-lg border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive">
    {errorMessage(error)}
    {retry && <Button variant="outline" size="sm" className="ml-3" onClick={retry}>重试</Button>}
  </div>
}
