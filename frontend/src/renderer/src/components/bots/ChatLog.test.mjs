import { expect, test } from 'bun:test'

test('bot chat renders pasted text and image attachments, including attachment-only messages', async () => {
  const worker = new globalThis.Worker(new globalThis.URL('./ChatLog.worker.mjs', import.meta.url).href)
  try {
    const result = await new Promise((resolve, reject) => {
      worker.onmessage = (event) => resolve(event.data)
      worker.onerror = reject
    })
    for (const html of result) {
      expect(html).toContain('pasted-text-1.txt')
      expect(html).toContain('alt="screenshot.png"')
      expect(html).toContain('/v1/sessions/bot-1/attachments/image-1')
    }
    expect(result[1]).toContain('Read these')
  } finally {
    worker.terminate()
  }
})
