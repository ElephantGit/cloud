import { test } from 'node:test'
import assert from 'node:assert/strict'
import { addTask } from '../src/tasks.mjs'

test('adds a task without mutating its input', () => {
  const tasks = []
  assert.deepEqual(addTask(tasks, '  准备演示  '), [{ id: 1, title: '准备演示' }])
  assert.deepEqual(tasks, [])
})

test('rejects an empty title', () => {
  assert.throws(() => addTask([], '  '), /title is required/)
})
