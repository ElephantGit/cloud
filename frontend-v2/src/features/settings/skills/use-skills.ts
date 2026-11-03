import { isAxiosError } from 'axios'
import { useQueryClient } from '@tanstack/react-query'
import { useGetApiV1MeTenants } from '@/api/me/me'
import {
  getListSkillsQueryKey,
  useCreateSkill,
  useDeleteSkill,
  useImportSkill,
  useListSkills,
  useUpdateSkill,
  type SkillError,
} from '@/lib/skills-api/generated'

function codeOf(error: unknown): string | undefined {
  if (!isAxiosError(error)) return undefined
  const body = error.response?.data as Partial<SkillError> | undefined
  return typeof body?.code === 'string' ? body.code : undefined
}

const messages: Record<string, string> = {
  forbidden: '你没有权限管理这个工作区的技能。',
  skill_not_found: '技能已不存在，请刷新列表。',
  skill_name_conflict: '已有同名技能，请改用其他名称或选择覆盖。',
  skill_file_type: '请选择 Markdown 格式的 SKILL.md 文件。',
  skill_file_too_large: 'SKILL.md 不能超过 1 MiB。',
  skill_front_matter_missing:
    'SKILL.md 缺少 YAML front matter，需要声明 name 和 description。',
  skill_front_matter_invalid: 'SKILL.md 的 YAML front matter 无法解析。',
  skill_name_missing: 'SKILL.md 缺少 name。',
  skill_name_invalid:
    '技能名称需为单个 ASCII 路径段，不能以点开头，也不能包含路径分隔符。',
  skill_description_missing: 'SKILL.md 缺少 description。',
  skill_description_too_large: '技能简介不能超过 4096 字节。',
  skill_content_too_large: '技能正文不能超过 900 KiB。',
  skill_content_invalid: '技能正文不是有效的 UTF-8 文本。',
}

export function skillError(error: unknown): unknown {
  const code = codeOf(error)
  if (code && messages[code]) return new Error(messages[code])
  if (isAxiosError(error) && error.response?.status === 401)
    return new Error('登录已失效，请重新登录。')
  return error
}

export function isSkillNameConflict(error: unknown) {
  return codeOf(error) === 'skill_name_conflict'
}

/**
 * The signed-in user's server-backed skill catalog. All reads and writes stay scoped to the first
 * workspace the session belongs to; market packages continue through the marketplace hook.
 */
export function useUserSkills() {
  const tenants = useGetApiV1MeTenants()
  const tenantId = tenants.data?.items?.[0]?.id ?? ''
  const client = useQueryClient()
  const query = useListSkills(tenantId, {
    query: { enabled: tenantId !== '', retry: false },
  })
  const invalidate = async () => {
    if (!tenantId) return
    await client.invalidateQueries({
      queryKey: getListSkillsQueryKey(tenantId),
    })
  }
  const create = useCreateSkill({
    mutation: { onSuccess: invalidate },
  })
  const update = useUpdateSkill({
    mutation: { onSuccess: invalidate },
  })
  const remove = useDeleteSkill({
    mutation: { onSuccess: invalidate },
  })
  const importFile = useImportSkill({
    mutation: { onSuccess: invalidate },
  })
  return {
    tenantId,
    query,
    skills: query.data?.items ?? [],
    create: create.mutateAsync,
    update: update.mutateAsync,
    remove: remove.mutateAsync,
    importFile: importFile.mutateAsync,
    savePending: create.isPending || update.isPending,
    deletePending: remove.isPending,
    loading: tenants.isLoading || query.isPending,
    error: skillError(tenants.error ?? query.error),
    mutationError: skillError(
      create.error ?? update.error ?? remove.error ?? importFile.error,
    ),
    resetErrors: () => {
      create.reset()
      update.reset()
      remove.reset()
      importFile.reset()
    },
    refetch: () => {
      void tenants.refetch()
      void query.refetch()
    },
  }
}
