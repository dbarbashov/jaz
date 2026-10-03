import { expect, mock, spyOn, test } from 'bun:test'
import { spawnSync } from 'node:child_process'
import { EventEmitter } from 'node:events'
import { fileURLToPath } from 'node:url'

if (process.env.JAZ_UPDATER_TEST_CHILD === '1') {
  const app = { isPackaged: true }
  const powerMonitor = new EventEmitter()
  const autoUpdater = new EventEmitter()
  const sent = []
  const timers = []
  let now = 0
  let checks = 0
  autoUpdater.checkForUpdates = () => {
    checks += 1
    return Promise.resolve(null)
  }
  mock.module('electron', () => ({ app, powerMonitor, autoUpdater: {}, ipcMain: {} }))
  mock.module('electron-updater', () => ({ autoUpdater }))
  mock.module('./backend', () => ({ terminateLocalBackend: async () => {} }))
  const { createUpdateController } = await import('./updater')

  function schedule(callback, delay, repeat) {
    timers.push({ callback, at: now + delay, repeat })
    return { unref() {} }
  }
  globalThis.setTimeout = (callback, delay) => schedule(callback, delay, 0)
  globalThis.setInterval = (callback, delay) => schedule(callback, delay, delay)

  async function advance(ms) {
    const target = now + ms
    for (;;) {
      const next = timers.filter((timer) => timer.at <= target).sort((a, b) => a.at - b.at)[0]
      if (!next) break
      now = next.at
      next.at = next.repeat ? next.at + next.repeat : Infinity
      next.callback()
      await Promise.resolve()
    }
    now = target
  }

  test('discovers updates every five minutes and after wake, retries failures and keeps ready updates', async () => {
    const controller = createUpdateController(() => ({ webContents: { send: (...args) => sent.push(args) } }))
    controller.start()
    controller.start()
    await advance(9_999)
    expect(checks).toBe(0)
    await advance(1)
    expect(checks).toBe(1)
    autoUpdater.emit('update-not-available')
    await advance(289_999)
    expect(checks).toBe(1)
    await advance(1)
    expect(checks).toBe(2)

    const log = spyOn(console, 'error').mockImplementation(() => {})
    autoUpdater.checkForUpdates = () => {
      checks += 1
      return Promise.reject(new Error('offline'))
    }
    try {
      await advance(300_000)
      expect(checks).toBe(3)
      expect(log).toHaveBeenCalledTimes(1)
    } finally {
      log.mockRestore()
    }
    autoUpdater.checkForUpdates = () => {
      checks += 1
      return Promise.resolve(null)
    }
    await advance(300_000)
    expect(checks).toBe(4)
    powerMonitor.emit('resume')
    expect(checks).toBe(5)
    autoUpdater.emit('update-available', { version: '0.0.159' })
    autoUpdater.emit('download-progress', { percent: 50 })
    await advance(300_000)
    powerMonitor.emit('resume')
    expect(checks).toBe(5)
    autoUpdater.emit('update-downloaded', { version: '0.0.159' })
    await advance(300_000)
    powerMonitor.emit('resume')
    controller.checkForUpdates()
    expect(checks).toBe(5)
    expect(sent.at(-1)).toEqual(['jaz:update-status', { state: 'downloaded', version: '0.0.159' }])
  })

  test('development apps do not start background update checks', () => {
    app.isPackaged = false
    const timerCount = timers.length
    const resumeCount = powerMonitor.listenerCount('resume')
    createUpdateController(() => null).start()
    expect(timers).toHaveLength(timerCount)
    expect(powerMonitor.listenerCount('resume')).toBe(resumeCount)
  })
} else {
  test('desktop updater scheduling in an isolated Electron fixture', () => {
    const result = spawnSync(process.execPath, ['test', fileURLToPath(import.meta.url)], {
      env: { ...process.env, JAZ_UPDATER_TEST_CHILD: '1' },
      encoding: 'utf8',
      timeout: 10_000,
    })
    expect({ status: result.status, error: result.error?.message, output: result.status ? result.stderr : '' })
      .toEqual({ status: 0, error: undefined, output: '' })
  })
}
