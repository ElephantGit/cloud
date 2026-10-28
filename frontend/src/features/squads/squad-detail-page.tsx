import { SquadsPage } from './squads-page'

/** Keeps old Squad detail URLs usable without resolving demo identities or displaying fake records. */
export function SquadDetailPage({ slug }: { slug: string }) {
  return <SquadsPage slug={slug} />
}
