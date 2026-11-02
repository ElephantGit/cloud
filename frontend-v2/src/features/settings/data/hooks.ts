import { useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useRole, useUser } from '@/app/auth/user'
import { DemoRepository, settingsDemoQueryKey } from './repository'
import type { Snapshot } from './types'

/** Demo and HTTP caches never share keys. Mutations commit storage before updating query data. */
export function useSettingsData() {
  const user = useUser()
  // Demo writes follow the real workspace role; the roster itself always comes from the server.
  const role = useRole()
  const client = useQueryClient()
  const repository = useMemo(
    () => new DemoRepository({ id: user.id }),
    [user.id],
  )
  const queryKey = settingsDemoQueryKey(repository)
  const query = useQuery({
    queryKey,
    queryFn: () => repository.read(),
    retry: false,
    staleTime: 0,
  })
  const mutation = useMutation({
    mutationFn: ({
      revision,
      change,
    }: {
      revision: number
      change: (data: Snapshot) => void
    }) => repository.update(revision, change),
    onSuccess: (data) => client.setQueryData(queryKey, data),
    onError: () => {
      void client.invalidateQueries({ queryKey })
    },
  })
  const save = (
    change: (data: Snapshot) => void,
    message: string,
    done?: () => void,
  ) => {
    if (!query.data || mutation.isPending) return
    mutation.mutate(
      { revision: query.data.revision, change },
      {
        onSuccess: () => {
          toast.success(message)
          done?.()
        },
      },
    )
  }
  const actor = { id: user.id, displayName: user.displayName, role: role ?? 'member' }
  return { user: actor, repository, queryKey, query, mutation, save, canManage: role === 'owner' }
}
