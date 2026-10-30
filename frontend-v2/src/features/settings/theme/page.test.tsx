import { beforeEach, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderSettingsPage } from '../test-utils'
import ThemePage from './page'

beforeEach(() => {
  localStorage.clear()
  document.documentElement.classList.remove('dark', 'light')
})
it('applies explicit dark/light preferences and persists system selection', async () => {
  const user = userEvent.setup()
  renderSettingsPage(<ThemePage />)
  await user.click(screen.getByRole('button', { name: '深色' }))
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
  expect(localStorage.getItem('ora-v2-theme')).toBe('dark')
  await user.click(screen.getByRole('button', { name: '浅色' }))
  await waitFor(() => expect(document.documentElement).not.toHaveClass('dark'))
  await user.click(screen.getByRole('button', { name: '跟随系统' }))
  expect(localStorage.getItem('ora-v2-theme')).toBe('system')
})
