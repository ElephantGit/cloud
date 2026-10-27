import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { newJoinToken, useCreateInvitation, useCreateJoinLink } from '@/features/members/api'
import { copyJoinLink, entryDate } from '@/features/members/browser'
import { joinUrl } from '@/lib/paths'

function useCreateLinkState({
  tenantId,
  kind,
  onClose,
}: {
  tenantId: string
  kind: 'invite' | 'apply'
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const invitation = useCreateInvitation(tenantId)
  const application = useCreateJoinLink(tenantId)
  const mutation = kind === 'invite' ? invitation : application
  const [note, setNote] = useState('')
  const [created, setCreated] = useState<{ url: string; expiresAt: string } | undefined>()
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'failed'>('idle')
  const [closing, setClosing] = useState(false)
  function requestClose() {
    if (mutation.isPending) return
    if (created) setClosing(true)
    else onClose()
  }
  function generate() {
    const token = newJoinToken()
    mutation.mutate(
      { token, note },
      { onSuccess: (row) => setCreated({ url: joinUrl(kind, token), expiresAt: row.expiresAt }) },
    )
  }
  async function copy() {
    if (!created) return
    try {
      await copyJoinLink(created.url)
      setCopyState('copied')
    } catch {
      setCopyState('failed')
    }
  }
  return {
    t,
    i18n,
    mutation,
    note,
    setNote,
    created,
    copyState,
    closing,
    setClosing,
    requestClose,
    generate,
    copy,
  }
}

/** Owns plaintext for one generation dialog only; closing unmounts and discards it. */
export function CreateLinkDialog(props: Parameters<typeof useCreateLinkState>[0]) {
  const { kind, onClose } = props
  const {
    t,
    i18n,
    mutation,
    note,
    setNote,
    created,
    copyState,
    closing,
    setClosing,
    requestClose,
    generate,
    copy,
  } = useCreateLinkState(props)
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) requestClose()
      }}
    >
      <DialogContent showCloseButton={false} className="sm:max-w-lg">
        <DialogTitle>
          {closing ? t('entry.closeTitle') : t('entry.generateTitle', { kind: t(`entry.${kind}`) })}
        </DialogTitle>
        <DialogDescription>
          {closing ? t('entry.closeHint') : t(`entry.${kind}Hint`)}
        </DialogDescription>
        {closing ? (
          <div className="flex flex-wrap gap-2">
            <Button onClick={onClose}>{t('entry.closeConfirm')}</Button>
            <Button variant="outline" onClick={() => setClosing(false)}>
              {t('entry.backCopy')}
            </Button>
          </div>
        ) : (
          <>
            {created ? (
              <>
                <p className="text-sm">
                  {t('entry.expiresAt')} · {entryDate(created.expiresAt, i18n.language)}
                </p>
                <Input
                  aria-label={t('entry.shareUrl')}
                  readOnly
                  value={created.url}
                  onFocus={(event) => event.target.select()}
                />
                <p className="text-sm text-muted-foreground">{t('entry.once')}</p>
                <Button onClick={() => void copy()}>
                  {t(copyState === 'copied' ? 'entry.copied' : 'entry.copy')}
                </Button>
                {copyState === 'failed' && (
                  <p role="alert" className="text-sm text-destructive">
                    {t('entry.copyFailed')}
                  </p>
                )}
                {copyState === 'copied' && (
                  <p role="status" className="sr-only">
                    {t('entry.copied')}
                  </p>
                )}
              </>
            ) : (
              <>
                <label className="space-y-2 text-sm">
                  {t('entry.note')}
                  <Input
                    value={note}
                    onChange={(event) => setNote(event.target.value)}
                    maxLength={200}
                    placeholder={t('entry.noteHint')}
                  />
                </label>
                <Button disabled={mutation.isPending} onClick={generate}>
                  {t(mutation.isPending ? 'entry.creating' : 'entry.generate')}
                </Button>
                {mutation.isError && (
                  <p role="alert" className="text-destructive">
                    {t('entry.actionFailed')}
                  </p>
                )}
              </>
            )}
            <Button variant="outline" disabled={mutation.isPending} onClick={requestClose}>
              {t(created ? 'entry.close' : 'entry.cancel')}
            </Button>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
