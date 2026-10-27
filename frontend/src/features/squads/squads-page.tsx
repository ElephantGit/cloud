import { UnavailablePage } from '@/features/unavailable/unavailable-page'
import { workspacePaths } from '@/lib/paths'

/** Directs members to the authoritative roster while Squad management is unavailable. */
export function SquadsPage({ slug }: { slug: string }) {
  return (
    <UnavailablePage
      title="小队"
      description="小队管理尚未开放。通过邀请加入工作区的成员，可以在“设置 → 成员”中查看。"
      actions={[{ label: '查看工作区成员', to: workspacePaths(slug).members }]}
    />
  )
}
