import { createContext, useContext } from 'react'
import type { User } from '@/api/generated.schemas'

export const UserContext = createContext<User | null>(null)

/** The signed-in user; only valid below `SessionGate`. */
export function useUser(): User {
  const user = useContext(UserContext)
  if (!user) throw new Error('useUser outside SessionGate')
  return user
}
