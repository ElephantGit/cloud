import assert from 'node:assert/strict'
import { test } from 'node:test'
import { advance, cancel, columns } from '../src/board.mjs'

test('columns keep every status and the input order', () => {
  const tasks = [
    { id: 1, status: 'review' },
    { id: 2, status: 'pending' },
    { id: 3, status: 'review' },
    { id: 4, status: 'cancelled' },
  ]
  const board = columns(tasks)
  assert.deepEqual(Object.keys(board), ['pending', 'running', 'review', 'done'])
  assert.deepEqual(board.review.map((t) => t.id), [1, 3])
  assert.equal(tasks.length, 4)
})

test('advance follows the lifecycle without mutating the task', () => {
  const task = { id: 1, status: 'pending' }
  assert.equal(advance(task).status, 'running')
  assert.equal(task.status, 'pending')
  assert.throws(() => advance({ id: 2, status: 'done' }))
})

test('cancel refuses finished tasks', () => {
  assert.equal(cancel({ id: 1, status: 'running' }).status, 'cancelled')
  assert.throws(() => cancel({ id: 2, status: 'done' }))
})
