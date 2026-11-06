import test from 'node:test'
import assert from 'node:assert/strict'
import { render } from '../src/report.mjs'

test('render fills the template from the values', () => {
  assert.equal(render('# ${values.title}\n\n共 ${values.count} 条', { title: '周报', count: 3 }), '# 周报\n\n共 3 条')
})
