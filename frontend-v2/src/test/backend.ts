import type { AxiosAdapter, AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import { http } from '@/lib/api-client'

type Route = (config: InternalAxiosRequestConfig) => { status: number; data?: unknown } | undefined

/**
 * Replaces the network with `route` for the shared axios instance; unmatched requests answer 404.
 * Returns the requests seen, as "METHOD /path".
 */
export function fakeBackend(route: Route): string[] {
  const seen: string[] = []
  const adapter: AxiosAdapter = async (config) => {
    seen.push(`${config.method?.toUpperCase()} ${config.url}`)
    const { status, data } = route(config) ?? { status: 404 }
    const response: AxiosResponse = { status, statusText: String(status), data, headers: {}, config }
    if (status >= 400) {
      const { AxiosError } = await import('axios')
      throw new AxiosError(`HTTP ${status}`, String(status), config, undefined, response)
    }
    return response
  }
  http.defaults.adapter = adapter
  return seen
}

export const demoUser = {
  id: '00000000-0000-4000-8000-000000000001',
  displayName: '演示用户',
  status: 'active',
  version: 1,
  createdAt: '2026-10-29T00:00:00Z',
  deletedAt: null,
}
