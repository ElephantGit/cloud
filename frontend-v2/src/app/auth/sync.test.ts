import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  AUTH_IDENTITY_KEY,
  forgetAuthIdentity,
  rememberAuthIdentity,
  subscribeAuthIdentity,
} from './sync'

describe('auth identity sync', () => {
  beforeEach(() => localStorage.clear())

  it('publishes an identity and removes it at sign-out', () => {
    rememberAuthIdentity('user-a')
    expect(localStorage.getItem(AUTH_IDENTITY_KEY)).toBe('user-a')

    forgetAuthIdentity()
    expect(localStorage.getItem(AUTH_IDENTITY_KEY)).toBeNull()
  })

  it('notifies only for identity changes from another tab', () => {
    const onChange = vi.fn()
    const stop = subscribeAuthIdentity(onChange)

    window.dispatchEvent(new StorageEvent('storage', { key: 'other', newValue: 'x' }))
    expect(onChange).not.toHaveBeenCalled()

    window.dispatchEvent(new StorageEvent('storage', { key: AUTH_IDENTITY_KEY, newValue: 'user-b' }))
    expect(onChange).toHaveBeenCalledOnce()

    stop()
    window.dispatchEvent(new StorageEvent('storage', { key: AUTH_IDENTITY_KEY, newValue: null }))
    expect(onChange).toHaveBeenCalledOnce()
  })
})
