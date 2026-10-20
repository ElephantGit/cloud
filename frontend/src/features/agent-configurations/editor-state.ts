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
import { agentConfigurationBody, agentConfigurationDraft, type AgentSettingValue } from './draft'

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
  const connections = useModelConnections()
  const globalDefault = useModelDefault(draft.usesGlobalDefault)
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
    selectConnection(connection.id, connection.models[0]?.id ?? '')
  }
  function submit(event: React.FormEvent) {
    event.preventDefault()
    if (canSave && body) save.mutate({ body, repair: draft.requiresRepair }, { onSuccess: onSaved })
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
    reload,
    choose,
    changeSetting,
    selectConnection,
    createdConnection,
    submit,
    clearOptions,
  }
}

/** State and named actions shared only by the editor's presentational pieces. */
export type AgentEditorState = ReturnType<typeof useAgentEditor>
