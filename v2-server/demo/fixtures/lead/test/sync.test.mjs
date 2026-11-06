import test from 'node:test'
import assert from 'node:assert/strict'
import { pullTasks } from '../src/sync.mjs'

test('pullTasks returns the JSON body from the board service', async () => {
  const original = globalThis.fetch
  globalThis.fetch = async () => ({ ok: true, json: async () => ({ items: [{ id: 't1' }] }) })
  try {
    assert.deepEqual(await pullTasks('http://demo'), { items: [{ id: 't1' }] })
  } finally {
    globalThis.fetch = original
  }
})
