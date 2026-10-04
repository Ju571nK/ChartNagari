import { describe, expect, it } from 'vitest'
import { FX_PRESETS, formatPrice, instrument, isForexSymbol, isUSDPair, visibleSession } from './forex'
import { demoFxBars, demoFxSessions, ny4HBucket } from './demoFx'
import { interpolatedTimeCoordinate, sessionPositions } from './ForexPanels'

describe('forex symbols and instruments', () => {
  it('offers the exact major and cross presets without duplicates', () => {
    expect(FX_PRESETS.majors).toEqual(['EURUSD', 'GBPUSD', 'USDJPY', 'USDCHF', 'AUDUSD', 'USDCAD', 'NZDUSD'])
    expect(FX_PRESETS.majorsGold).toHaveLength(8)
    expect(new Set(FX_PRESETS.all28).size).toBe(28)
    expect(FX_PRESETS.all28.every(isForexSymbol)).toBe(true)
  })
  it('rejects invalid pairs and sets pip/price precision', () => {
    for (const symbol of ['EURUSD', 'CHFJPY', 'XAUUSD', 'XAGUSD']) expect(isForexSymbol(symbol)).toBe(true)
    for (const symbol of ['EURUS', 'EURUSDT', 'FOOUSD', 'USDEUR1', 'XAUJPY', 'EUR EUR']) expect(isForexSymbol(symbol)).toBe(false)
    expect(isUSDPair('EURUSD')).toBe(true)
    expect(isUSDPair('EURGBP')).toBe(false)
    expect(instrument('USDJPY').pip).toBe(0.01)
    expect(instrument('XAUUSD').pip).toBe(0.1)
    expect(formatPrice('EURUSD', 1.0842)).toBe('1.08420')
  })
})

describe('session geometry', () => {
  it('uses actual time and price coordinates and hides daily/weekly', () => {
    expect(interpolatedTimeCoordinate([10, 20, 30], 15, t => t * 2)).toBe(30)
    expect(interpolatedTimeCoordinate([10, 20, 30], 35, t => t * 2)).toBe(70)
    const boxes = sessionPositions([{ name: 'asia', start: 10, end: 20, high: 5, low: 3, asian_high: 5, asian_low: 3 }], t => t * 2, p => 100 - p * 10)
    expect(boxes).toEqual([{ name: 'asia', left: 20, width: 20, top: 50, height: 20, highY: 50, lowY: 70 }])
    expect(visibleSession('1H')).toBe(true)
    expect(visibleSession('4H')).toBe(true)
    expect(visibleSession('1D')).toBe(false)
    expect(visibleSession('1W')).toBe(false)
  })
  it('moves New York local sessions with DST in the demo fixture', () => {
    const bars = demoFxBars('EURUSD', '1H')
    const sessions = demoFxSessions('EURUSD', bars[0].time, bars.at(-1)!.time)
    const asian = sessions.filter(s => s.name === 'asia')
    expect(asian.length).toBeGreaterThan(2)
    expect(asian.every(s => s.asian_high === s.high && s.asian_low === s.low)).toBe(true)
    const offsets = new Set(asian.map(s => new Date(s.start * 1000).getUTCHours()))
    expect(offsets.size).toBeGreaterThan(1)
  })
  it('anchors complete 4H candles to New York 17:00 across DST', () => {
    expect(new Date(ny4HBucket(Date.parse('2026-10-31T22:00:00Z') / 1000) * 1000).toISOString()).toBe('2026-10-31T21:00:00.000Z')
    expect(new Date(ny4HBucket(Date.parse('2026-11-02T23:00:00Z') / 1000) * 1000).toISOString()).toBe('2026-11-02T22:00:00.000Z')
    const hourly = demoFxBars('EURUSD', '1H')
    const bars = demoFxBars('EURUSD', '4H')
    expect(bars.length).toBeGreaterThan(20)
    for (const bar of bars) {
      const members = hourly.filter(hour => ny4HBucket(hour.time) === bar.time)
      expect(members).toHaveLength(4)
      expect(members.map(hour => hour.time)).toEqual([0, 1, 2, 3].map(offset => bar.time + offset * 3600))
      expect(bar.open).toBe(members[0].open)
      expect(bar.close).toBe(members[3].close)
    }
  })
})
