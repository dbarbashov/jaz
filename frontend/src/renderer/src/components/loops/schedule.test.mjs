import { expect, test } from 'bun:test'
import { compactSchedule } from './schedule'

test('schedules read as plain words, and odd ones leave the next run to speak', () => {
  const cases = {
    '0 */2 * * *': 'Every 2 hours',
    '*/15 * * * *': 'Every 15 minutes',
    '0 * * * *': 'Hourly',
    '0 9-18/2 * * 1-5': 'Weekdays · Every 2 hours, 9 AM–6 PM',
    '0 9-17 * * *': 'Hourly, 9 AM–5 PM',
    '30 9 * * *': 'Daily · 9:30 AM',
    '0 9,17 * * 1-5': 'Weekdays · 9:00 AM, 5:00 PM',
    '0 8 * * 1': 'Mondays · 8:00 AM',
    '0 9 1 * *': '',
    '5 4 * * 2,4': '',
  }
  for (const [expr, label] of Object.entries(cases)) {
    expect(compactSchedule(expr, false)).toBe(label)
  }
  expect(compactSchedule('0 */2 * * *', true)).toBe('Manual')
})
