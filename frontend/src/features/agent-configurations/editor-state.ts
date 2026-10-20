import { useState } from 'react'
import {
  useModelConnections,
  useModelDefault,
  type ModelConnection,
} from '@/features/model-connections/api'
import {
  useSaveAgentConfiguration,
  useClearAgentConfiguration,
  type AgentConfiguration,
  type AgentConfigurationScope,
} from './api'
import {
  agentConfigurationBody,
  agentConfigurationDraft,
  agentConfigurationNumbers,
  type AgentConfigurationDraft,
  type AgentSettingValue,
} from './draft'

/**
 * Resolves the connection whose write-only credential controls this session shows.
 * Global-default plugins persist no connection, so the default draft or an inline
 * creation names the target until the user chooses another connection there.
 */
function useCredentialTarget(draft: AgentConfigurationDraft) {
  const [focus, setFocus] = useState<string>()
  const connections = useModelConnections()
  const globalDefault = useModelDefault(draft.usesGlobalDefault)
  const connectionId = draft.usesGlobalDefault
    ? (focus ?? globalDefault.data?.connectionId)
    : draft.connectionId
  return {
    connections,
    globalDefault,
    target: connections.data?.find((connection) => connection.id === connectionId),
    focus: setFocus,
  }
}

/** Input to an editing session; successful writes are the only implicit remount boundary. */
export type AgentEditorProps = {
  context: AgentConfiguration
  scope: AgentConfigurationScope
  onReload: () => Promise<void>
  onSaved: () => void
}

/** Owns draft baselines and explicit reconciliation without copying live server state. */
export function useAgentEditor({ context, scope, onReload, onSaved }: AgentEditorProps) {
  const [draft, setDraft] = useState(() => agentConfigurationDraft(context))
  const [acceptedRepair, setAcceptedRepair] = useState(false)
  const [comparing, setComparing] = useState(false)
  const [confirmClear, setConfirmClear] = useState(false)
  const [reloadFailed, setReloadFailed] = useState(false)
  const save = useSaveAgentConfiguration(scope)
  const clear = useClearAgentConfiguration(scope)
  const { connections, globalDefault, target, focus } = useCredentialTarget(draft)
  const body = agentConfigurationBody(draft)
  const busy = save.isPending || clear.isPending
  const canSave =
    !busy && body !== undefined && (!draft.requiresRepair || acceptedRepair) && !comparing
  async function reload() {
    setReloadFailed(false)
    try {
      await onReload()
      setComparing(true)
    } catch {
      setReloadFailed(true)
    }
  }
  function choose(keepDraft: boolean) {
    setDraft(agentConfigurationDraft(context, keepDraft ? draft : undefined))
    setAcceptedRepair(false)
    setComparing(false)
    save.reset()
    clear.reset()
  }
  function changeSetting(id: string, value: AgentSettingValue | undefined) {
    setDraft((current) => {
      const values = { ...current.values }
      if (value === undefined) delete values[id]
      else values[id] = value
      return { ...current, values }
    })
  }
  function selectConnection(connectionId: string, modelId: string) {
    setDraft((current) => ({ ...current, connectionId, modelId }))
  }
  function createdConnection(connection: ModelConnection) {
    if (draft.usesGlobalDefault) {
      focus(connection.id)
      return
    }
    selectConnection(connection.id, connection.models[0]?.id ?? '')
  }
  function submit(event: React.FormEvent) {
    event.preventDefault()
    if (canSave && body)
      save.mutate(
        { body, repair: draft.requiresRepair, numbers: agentConfigurationNumbers(draft) },
        { onSuccess: onSaved },
      )
  }
  function clearOptions() {
    clear.mutate({ revision: draft.revision }, { onSuccess: onSaved })
  }
  return {
    draft,
    body,
    busy,
    canSave,
    acceptedRepair,
    setAcceptedRepair,
    comparing,
    confirmClear,
    setConfirmClear,
    reloadFailed,
    save,
    clear,
    connections,
    globalDefault,
    credentialTarget: target,
    reload,
    choose,
    changeSetting,
    selectConnection,
    createdConnection,
    followDefaultConnection: focus,
    submit,
    clearOptions,
  }
}

/** State and named actions shared only by the editor's presentational pieces. */
export type AgentEditorState = ReturnType<typeof useAgentEditor>
