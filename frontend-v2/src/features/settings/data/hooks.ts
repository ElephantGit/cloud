import { useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useUser } from '@/app/auth/user'
import { DemoRepository } from './repository'
import type { Snapshot } from './types'

/** Demo and HTTP caches never share keys. Mutations commit storage before updating query data. */
export function useSettingsData() {
  const user = useUser()
  const client = useQueryClient()
  const repository = useMemo(
    () => new DemoRepository({ id: user.id, displayName: user.displayName }),
    [user.id, user.displayName],
  )
  const queryKey = ['settings-demo', repository.key]
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
  const reset = useMutation({
    mutationFn: () => repository.reset(),
    onSuccess: (data) => {
      client.setQueryData(queryKey, data)
      toast.success('演示数据已重置')
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
  const canManage = !!query.data?.members.some(
    (m) => m.userId === user.id && m.role === 'admin' && m.status === 'active',
  )
  return { user, repository, queryKey, query, mutation, reset, save, canManage }
}
