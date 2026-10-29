import { isAxiosError } from 'axios'

export const fieldClass = 'w-full rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:border-foreground/50 focus:ring-2 focus:ring-foreground/10 disabled:opacity-60'

export function errorMessage(error: unknown): string {
  if (!isAxiosError(error)) return error instanceof Error ? error.message : '操作失败，请重试。'
  const code = error.response?.data?.code
  const messages: Record<string, string> = {
    workflow_conflict: '这个名称已被使用，请换一个名称。',
    version_conflict: '草稿已有新版本。你的修改仍保留，请先导出备份，再重新载入最新草稿。',
    version_required: '缺少草稿版本，请重新载入后再试。',
    workflow_graph_invalid: '节点或连线结构不完整，请检查流程内容后再发布。',
    not_found: '工作流或版本不存在，可能已被归档。',
    forbidden: '你没有访问这个团队的权限。',
    unauthenticated: '登录已过期，请刷新页面重新登录。',
    invalid_input: '内容格式不正确，请检查名称和流程文档。',
  }
  return messages[code] ?? '请求失败，请检查连接后重试。'
}

/** Download the document without changing its authored fields. */
export function downloadGraph(name: string, graph: string) {
  const url = URL.createObjectURL(new Blob([graph], { type: 'application/json' }))
  const link = document.createElement('a')
  link.href = url
  link.download = `${name || 'workflow'}.json`
  link.click()
  URL.revokeObjectURL(url)
}
