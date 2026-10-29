import { http } from './api-client'

// The Gateway's browser login routes. They are not part of api/openapi.json, so they are written by
// hand; v2-server serves the same shapes with only the development provider.

export interface Providers {
  providers: string[]
  default?: string
}

export const getProviders = () => http.get<Providers>('/auth/providers').then((r) => r.data)

/** Starts a login; the browser then navigates to the returned URL. */
export const startLogin = (provider: string, returnTo: string) =>
  http.post<{ authorizationUrl: string }>('/auth/login', { provider, returnTo }).then((r) => r.data.authorizationUrl)

export const logout = () => http.post('/auth/logout').then(() => undefined)
