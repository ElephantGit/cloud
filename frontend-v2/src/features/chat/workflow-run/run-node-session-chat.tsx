import { ChevronRightIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { Bubble, BubbleContent } from '@/components/ui/bubble'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Message, MessageContent } from '@/components/ui/message'
import { subtleScrollbar } from '@/lib/scrollbar'
import { cn } from '@/lib/utils'
import type { WorkflowNodeConversationItem } from '@/lib/workflows/runtime'
import { Markdown } from '../components/markdown'

/**
 * An Agent node's conversation in the chat page's message style. Desktop shows the node's real
 * session here through its chat view; the demo engine keeps only the conversation items, so this
 * reads them: what was sent to the Agent on the right, its replies on the left, and its tool and
 * thinking steps behind one collapsed line, as desktop's node card does.
 */
export function RunNodeSessionChat({ conversation, sessionActions }: { conversation: WorkflowNodeConversationItem[]; sessionActions?: ReactNode }) {
  const activity = conversation.filter((item) => item.kind === 'activity')
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className={cn('flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto py-2', subtleScrollbar)}>
        {activity.length > 0 && (
          <Collapsible>
            <CollapsibleTrigger className="group/trigger flex items-center gap-1 text-xs text-muted-foreground">
              <ChevronRightIcon className="size-3 transition-transform group-aria-expanded/trigger:rotate-90" />
              活动 {activity.length} 项
            </CollapsibleTrigger>
            <CollapsibleContent>
              <ul className="mt-1 flex flex-col gap-1 border-l pl-3 text-xs text-muted-foreground">
                {activity.map((item) => item.kind === 'activity' && <li key={item.id}>{item.summary}</li>)}
              </ul>
            </CollapsibleContent>
          </Collapsible>
        )}
        {conversation.map((item) =>
          item.kind !== 'message' ? null : item.role === 'user' ? (
            <Message key={item.id} align="end" aria-label="发给 Agent 的内容">
              <MessageContent>
                <Bubble align="end" variant="muted">
                  <BubbleContent className="whitespace-pre-wrap">{item.markdown}</BubbleContent>
                </Bubble>
              </MessageContent>
            </Message>
          ) : (
            <div key={item.id} aria-label="Agent 回复" className="text-sm">
              <Markdown text={item.markdown} />
            </div>
          ),
        )}
      </div>
      {sessionActions && <div className="flex shrink-0 items-center pt-1">{sessionActions}</div>}
    </div>
  )
}
