import { Navigate, useNavigate, useParams, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { ShieldCheck, Link2Off } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { JoinLinkPreview } from '@/api/generated.schemas'
import { useSession } from '@/features/auth/session'
import { useJoinPreview, useRedeemInvitation, useRequestJoin } from '@/features/onboarding/api'
import {
  clearPendingJoin,
  pendingJoinPath,
  rememberJoinPath,
} from '@/features/onboarding/join-navigation'
import { loginPath, workspacePaths } from '@/lib/paths'
import { faultCode, faultParam } from '@/lib/api-client'

/** Restores a tab-private bearer path after OAuth without ever giving it to Gateway. */
export function JoinContinue() {
  return <Navigate to={pendingJoinPath() ?? '/onboarding'} replace />
}

/** Checks anonymous link validity before login, then requests conscious authenticated admission. */
export function JoinPage({ kind }: { kind: 'invite' | 'apply' }) {
  const { token = '' } = useParams<{ token: string }>()
  const { session } = useSession()
  const preview = useJoinPreview(
    kind,
    token,
    session.status === 'signed-in' ? session.user.id : session.status,
  )
  const invitation = useRedeemInvitation()
  const application = useRequestJoin()
  const navigate = useNavigate()
  const location = useLocation()
  const error = invitation.error ?? application.error
  const joined = !!invitation.data || preview.data?.alreadyMember === true
  const pending = !!application.data || preview.data?.applicationStatus === 'pending'
  const slug = preview.data?.slug
  const view = previewState(preview, token, error)
  function submit() {
    if (session.status === 'signed-out') {
      const remembered = rememberJoinPath(location.pathname)
      void navigate(loginPath(remembered ? '/join/continue' : '/onboarding'))
      return
    }
    if (session.status !== 'signed-in' || view.state !== 'active') return
    if (kind === 'invite') invitation.mutate(token, { onSuccess: clearPendingJoin })
    else application.mutate(token, { onSuccess: clearPendingJoin })
  }
  return (
    <JoinView
      kind={kind}
      {...view}
      joined={joined}
      pending={pending}
      busy={invitation.isPending || application.isPending || session.status === 'loading'}
      signedIn={session.status === 'signed-in'}
      blocked={blockedSession(session.status)}
      name={preview.data?.name}
      onSubmit={submit}
      onRetry={() => {
        invitation.reset()
        application.reset()
        void preview.refetch()
      }}
      onSpaces={() => {
        clearPendingJoin()
        void navigate(session.status === 'signed-in' ? '/onboarding' : loginPath('/onboarding'))
      }}
      onEnter={
        slug
          ? () => {
              clearPendingJoin()
              void navigate(workspacePaths(slug).issues)
            }
          : undefined
      }
    />
  )
}

function JoinView({
  kind,
  state,
  checking,
  failed,
  joined,
  pending,
  busy,
  signedIn,
  blocked,
  name,
  onSubmit,
  onRetry,
  onSpaces,
  onEnter,
}: {
  kind: 'invite' | 'apply'
  state: JoinLinkPreview['state'] | undefined
  checking: boolean
  failed: boolean
  joined: boolean
  pending: boolean
  busy: boolean
  signedIn: boolean
  blocked: 'entry.disabledAccount' | 'entry.authFailed' | undefined
  name: string | undefined
  onSubmit: () => void
  onRetry: () => void
  onSpaces: () => void
  onEnter: (() => void) | undefined
}) {
  const { t } = useTranslation()
  const active = state === 'active' && !failed
  const [title, hint] = joinCopy({ kind, state, checking, failed, joined, pending, name }, t)
  const actionable = canSubmit({ checking, active, joined, pending, blocked })
  const action = joinAction(kind, signedIn)
  return (
    <main className="flex min-h-svh items-center justify-center bg-muted/20 p-4">
      <section
        className="w-full max-w-md space-y-5 rounded-xl border bg-card p-6 shadow-sm"
        aria-live="polite"
      >
        {active || joined || pending ? (
          <ShieldCheck className="size-8 text-muted-foreground" />
        ) : (
          <Link2Off className="size-8 text-muted-foreground" />
        )}
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">Ora · {t(`entry.${kind}`)}</p>
          <h1 className="text-xl font-semibold">{title}</h1>
          {active && name && !joined && <p className="font-medium">{name}</p>}
          <p className="text-sm leading-relaxed text-muted-foreground">{hint}</p>
        </div>
        {blocked && (
          <p role="alert" className="text-sm text-destructive">
            {t(blocked)}
          </p>
        )}
        <div className="flex flex-col gap-2">
          {actionable && (
            <Button disabled={busy} onClick={onSubmit}>
              {t(busy ? 'entry.processing' : action)}
            </Button>
          )}
          {joined && onEnter && <Button onClick={onEnter}>{t('entry.enter')}</Button>}
          {failed && <Button onClick={onRetry}>{t('entry.retry')}</Button>}
          <Button variant="outline" onClick={onSpaces}>
            {t('entry.mySpaces')}
          </Button>
        </div>
      </section>
    </main>
  )
}

function previewState(preview: ReturnType<typeof useJoinPreview>, token: string, error: unknown) {
  const reason = unavailableReason(faultParam(error, 'reason'))
  const invalidToken =
    !/^[A-Za-z0-9_-]{43}$/.test(token) || faultCode(preview.error) === 'invalid_join_token'
  return {
    state: invalidToken ? ('unavailable' as const) : (reason ?? preview.data?.state),
    checking: !invalidToken && preview.isPending,
    failed: (!invalidToken && preview.isError) || (!!error && !reason),
  }
}

function canSubmit(view: {
  checking: boolean
  active: boolean
  joined: boolean
  pending: boolean
  blocked: string | undefined
}) {
  return !view.checking && view.active && !view.joined && !view.pending && !view.blocked
}

function blockedSession(status: string) {
  if (status === 'disabled') return 'entry.disabledAccount'
  if (status === 'unavailable') return 'entry.authFailed'
  return undefined
}

function joinAction(kind: 'invite' | 'apply', signedIn: boolean) {
  if (signedIn) return kind === 'invite' ? 'entry.confirmJoin' : 'entry.submitApply'
  return kind === 'invite' ? 'entry.loginInvite' : 'entry.loginApply'
}

function joinCopy(
  view: Pick<
    Parameters<typeof JoinView>[0],
    'kind' | 'state' | 'checking' | 'failed' | 'joined' | 'pending' | 'name'
  >,
  t: TFunction,
): [string, string] {
  if (view.joined) return [t('entry.alreadyJoined'), view.name ?? '']
  if (view.pending) return [t('entry.pendingTitle'), t('entry.pendingHint')]
  if (view.checking) return [t('entry.inspecting'), '']
  if (view.failed) return [t('entry.checkFailed'), t('entry.checkFailedHint')]
  if (view.state === 'active')
    return [
      t(view.kind === 'invite' ? 'entry.validInvite' : 'entry.validApply'),
      t(`entry.${view.kind}Hint`),
    ]
  const state = unavailableReason(view.state) ?? 'unavailable'
  return [t(`entry.${state}Title`), t(`entry.${state}Hint`)]
}

function unavailableReason(value: string | undefined) {
  return value === 'used' || value === 'expired' || value === 'revoked' || value === 'unavailable'
    ? value
    : undefined
}
