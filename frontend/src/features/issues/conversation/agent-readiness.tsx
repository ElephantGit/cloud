import { Link } from 'react-router-dom'
import type { TaskAgentReadiness } from '@/api/generated.schemas'
import { taskConversationFailureCode } from './api'

/** Shows only safe current-user configuration gaps; it grants no authority to append another model. */
export function AgentReadiness({
  readiness,
  settingsPath,
}: {
  readiness: TaskAgentReadiness | undefined
  settingsPath: string | undefined
}) {
  if (!readiness) return null
  if (readiness.errorCode)
    return (
      <div role="alert" className="space-y-1 text-xs text-destructive">
        <p>{taskConversationFailureCode(readiness.errorCode)}</p>
        {readiness.requiresPersonalModel &&
          readiness.errorCode !== 'issue_project_required' &&
          settingsPath && (
            <Link className="underline" to={settingsPath} target="_blank" rel="noopener noreferrer">
              配置个人模型
            </Link>
          )}
      </div>
    )
  if (readiness.requiresPersonalModel)
    return <p className="text-xs text-muted-foreground">后续新运行使用你的个人模型配置。</p>
  return null
}
