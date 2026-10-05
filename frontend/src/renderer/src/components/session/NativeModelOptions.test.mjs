import { expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { NativeModelOptions } from '@/components/session/NativeModelOptions'

const options = [
  {
    id: 'fast-mode', name: 'Fast mode', category: 'model_config', current_value: 'on',
    description: '1.5x speed, increased usage',
    options: [{ value: 'off', name: 'Off' }, { value: 'on', name: 'On' }],
  },
]

test('the running composer keeps Fast Mode available without fixed speed or usage claims', () => {
  const html = renderToStaticMarkup(createElement(NativeModelOptions, {
    options, running: true, pending: false, onChange() {},
  }))
  const buttons = html.match(/<button\b[^>]*>/g)
  const fast = buttons.find((button) => button.includes('role="switch"'))
  expect(fast).toContain('aria-label="Fast Mode"')
  expect(fast).toContain('aria-checked="true"')
  expect(fast).not.toMatch(/\sdisabled(?:=|>)/)
  expect(html).not.toContain('1.5x')
  expect(html).not.toContain('increased usage')
})

test('Fast Mode waits for acknowledgement and disappears when the provider withdraws it', () => {
  const pending = renderToStaticMarkup(createElement(NativeModelOptions, {
    options, running: true, pending: true, onChange() {},
  }))
  expect(pending.match(/<button\b[^>]*role="switch"[^>]*>/)[0]).toMatch(/\sdisabled(?:=|>)/)
  const withdrawn = renderToStaticMarkup(createElement(NativeModelOptions, {
    options: options.slice(1), running: true, pending: false, onChange() {},
  }))
  expect(withdrawn).not.toContain('Fast Mode')
  expect(withdrawn).not.toContain('role="switch"')
})
