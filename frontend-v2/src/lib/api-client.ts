import axios, { isAxiosError, type AxiosError, type AxiosRequestConfig } from 'axios'

/**
 * The axios instance behind every generated hook in `src/api` and the hand-written auth calls.
 * Authentication is the HttpOnly session cookie the browser sends on same-origin requests; the
 * frontend never sees a token.
 */
export const http = axios.create({ baseURL: '' })

// Cloud rejects POST and DELETE without an idempotency key; send one so switching from v2-server
// to Cloud changes nothing here.
http.interceptors.request.use(
  (config) => {
    if ((config.method === 'post' || config.method === 'delete') && !config.headers.get('Idempotency-Key')) {
      config.headers.set('Idempotency-Key', crypto.randomUUID())
    }
    return config
  },
  undefined,
  { synchronous: true },
)

/** Mutator orval calls for every generated request. */
export const customInstance = <T>(config: AxiosRequestConfig, options?: AxiosRequestConfig): Promise<T> =>
  http({ ...config, ...options }).then(({ data }) => data as T)

export type ErrorType<E> = AxiosError<E>
export type BodyType<B> = B

/** HTTP status of a failed request, if it reached the server. */
export const statusOf = (error: unknown): number | undefined => (isAxiosError(error) ? error.response?.status : undefined)
