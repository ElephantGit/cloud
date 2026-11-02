import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useUser } from '@/app/auth/user'
import { DemoRepository, settingsDemoQueryKey } from './repository'

/** Reads the roles saved by Settings; workflow pickers share this cache and user boundary. */
export function useRoleCatalog() {
  const user = useUser()
  const repository = useMemo(
    () => new DemoRepository({ id: user.id }),
    [user.id],
  )
  return useQuery({
    queryKey: settingsDemoQueryKey(repository),
    queryFn: () => repository.read().then((snapshot) => snapshot.roles),
    retry: false,
    staleTime: 0,
  })
}
