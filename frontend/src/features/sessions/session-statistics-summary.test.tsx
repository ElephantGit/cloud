import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { expect, it } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { server } from '@/test/msw-server'
import type { SessionModelStatistics } from '@/api/generated.schemas'
import { SessionStatisticsSummary } from './session-statistics-summary'

function statistics(): SessionModelStatistics {
  const known = { value: 0, knownCount: 1, totalCount: 2, availability: 'partial' as const }
  const unknown = {
    value: null,
    knownCount: 0,
    totalCount: 2,
    availability: 'unavailable' as const,
  }
  return {
    schemaVersion: 1,
    source: 'model_gateway',
    scope: 'recorded_http_attempts',
    inputSemantics: 'unknown',
    counts: {
      total: 2,
      started: 1,
      succeeded: 1,
      failed: 0,
      cancelled: 0,
      interrupted: 0,
      incomplete: 0,
    },
    firstRecordedAt: null,
    lastRecordedAt: null,
    usage: {
      inputTokens: known,
      outputTokens: unknown,
      totalTokens: unknown,
      cacheReadTokens: unknown,
      cacheWriteTokens: unknown,
      reasoningTokens: unknown,
    },
    timing: { meanDurationMs: known, p90DurationMs: unknown, meanFirstOutputMs: unknown },
  }
}

it('requests one selected aggregate without fetching model-call pages and hides stale values after denial', async () => {
  const requested: string[] = []
  let denied = false
  server.use(
    http.get('/api/v1/tenants/t1/issues/i1/runs/:rid/statistics', ({ params }) => {
      requested.push(String(params['rid']))
      return denied
        ? HttpResponse.json({ code: 'forbidden' }, { status: 403 })
        : HttpResponse.json(statistics())
    }),
  )
  const user = userEvent.setup()
  renderWithProviders(
    <SessionStatisticsSummary
      sessionRef={{ tid: 't1', issueId: 'i1', runId: 'selected' }}
      detailsHref="/analysis"
    />,
  )
  expect(await screen.findByText('0ms')).toBeInTheDocument()
  expect(screen.getAllByText('未知')).toHaveLength(2)
  expect(screen.getAllByText(/部分数据或未终结快照/)).toHaveLength(2)
  expect(requested).toEqual(['selected'])
  denied = true
  await user.click(screen.getByRole('button', { name: '刷新统计' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('统计暂不可用')
  expect(screen.queryByText('0ms')).not.toBeInTheDocument()
  expect(requested).toEqual(['selected', 'selected'])
})
