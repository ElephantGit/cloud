import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { handlers } from '@/mocks/handlers/index'

/**
 * Baseline: the mock-domain handlers plus a signed-out session, so any
 * render sees the same answer a browser without a gateway cookie gets. Tests
 * that need a member install the session fixtures from `cloud-handlers`.
 */
const signedOut = http.get('/api/v1/me', () =>
  HttpResponse.json({ code: 'unauthenticated', params: {}, requestId: 'r' }, { status: 401 }),
)

/** A production-shaped gateway: GitHub is the only login it offers. */
const githubOnly = http.get('/auth/providers', () =>
  HttpResponse.json({ providers: ['github'], default: 'github' }),
)

const previewsUnavailable = http.get(
  '/api/v1/tenants/:tid/issues/:iid/runs/:rid/thread/preview/events',
  () =>
    HttpResponse.json(
      { code: 'not_found', params: {}, requestId: 'preview-unavailable' },
      { status: 404 },
    ),
)

/**
 * Baseline answer for the workflow node session read: a settled node whose attempt left no
 * replayable record. Selecting a node on a run page issues this read, and the panel must report
 * the attempt's real state rather than the absence of a handler; tests that need records override
 * it with `server.use`.
 */
const nodeSessionUnavailable = http.get(
  '/api/v1/tenants/:tid/workflows/:wfid/runs/:rid/nodes/:nodeId/session',
  () =>
    HttpResponse.json({
      execution: {
        attempt: 1,
        executionId: null,
        result: null,
        records: [],
        state: 'unavailable',
        incomplete: false,
        hasMore: false,
        nextBefore: null,
      },
      node: { status: 'succeeded', attempt: 1, error: null, output: null, sessionEndReason: null },
    }),
)

export const server = setupServer(
  ...handlers,
  signedOut,
  githubOnly,
  previewsUnavailable,
  nodeSessionUnavailable,
)
