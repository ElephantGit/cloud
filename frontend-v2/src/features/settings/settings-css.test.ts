import { transform } from 'lightningcss'
import { expect, test } from 'vitest'
import source from './settings.css?raw'

test('the minified detail panel keeps the reset of the dialog centering', () => {
  const { code } = transform({
    filename: 'settings.css',
    code: new TextEncoder().encode(source),
    minify: true,
  })
  const rule = new TextDecoder()
    .decode(code)
    .match(/\.settings-detail\[data-slot=dialog-content\]\{[^}]*\}/)?.[0]
  expect(rule).toContain('translate:none')
})
