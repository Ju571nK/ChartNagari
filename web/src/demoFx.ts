import type { ForexSession } from './forex'

export interface DemoFxBar { time: number; open: number; high: number; low: number; close: number; volume: number }

const hourMs = 3600_000
const start = Date.UTC(2026, 9, 28, 0)
const nyFormatter = new Intl.DateTimeFormat('en-US', { timeZone: 'America/New_York', year: 'numeric', month: '2-digit', day: '2-digit', weekday: 'short', hour: '2-digit', hourCycle: 'h23' })
function nyParts(time: number) {
  const parts = Object.fromEntries(nyFormatter.formatToParts(new Date(time * 1000)).map(part => [part.type, part.value]))
  return { hour: Number(parts.hour), day: `${parts.year}-${parts.month}-${parts.day}`, weekday: parts.weekday }
}

function ny17(day: string): number {
  const midnight = Date.parse(`${day}T00:00:00Z`) / 1000
  for (let hour = 20; hour <= 23; hour++) {
    const candidate = midnight + hour * 3600
    const local = nyParts(candidate)
    if (local.day === day && local.hour === 17) return candidate
  }
  throw new Error(`NY session anchor unavailable: ${day}`)
}

export function ny4HBucket(time: number): number {
  const local = nyParts(time)
  const priorDay = new Date(Date.parse(`${local.day}T00:00:00Z`) - 86400_000).toISOString().slice(0, 10)
  const anchor = ny17(local.hour < 17 ? priorDay : local.day)
  return anchor + Math.floor((time - anchor) / (4 * 3600)) * 4 * 3600
}

const startingPrices: Record<string, number> = { EURUSD: 1.0842, GBPUSD: 1.298, USDJPY: 149.5, USDCHF: 0.908, AUDUSD: 0.676, USDCAD: 1.367, NZDUSD: 0.612, XAUUSD: 2745 }
function hourly(symbol: string): DemoFxBar[] {
  const result: DemoFxBar[] = []
  const gold = symbol === 'XAUUSD'
  let last = startingPrices[symbol] ?? 1.0842
  for (let i = 0; i < 260; i++) {
    const time = (start + i * hourMs) / 1000
    const ny = nyParts(time)
    if (ny.weekday === 'Sat' || (ny.weekday === 'Sun' && ny.hour < 17) || (ny.weekday === 'Fri' && ny.hour >= 17)) continue
    const scale = gold ? 2.4 : symbol.endsWith('JPY') ? 0.065 : 0.00042
    const step = (Math.sin(i * 0.38 + symbol.length) + Math.cos(i * 0.13) * 0.5) * scale
    const close = last + step
    result.push({ time, open: last, high: Math.max(last, close) + scale * 0.5, low: Math.min(last, close) - scale * 0.45, close, volume: 0 })
    last = close
  }
  return result.slice(-200)
}
const hourlyCache: Record<string, DemoFxBar[]> = Object.fromEntries(Object.keys(startingPrices).map(symbol => [symbol, hourly(symbol)]))

export function demoFxBars(symbol: string, timeframe: string): DemoFxBar[] {
  const bars = hourlyCache[symbol]
  if (timeframe === '1H') return bars
  if (timeframe === '4H') {
    const grouped = new Map<number, DemoFxBar[]>()
    for (const bar of bars) {
      const key = ny4HBucket(bar.time)
      grouped.set(key, [...(grouped.get(key) ?? []), bar])
    }
    return [...grouped].flatMap(([time, group]) => {
      if (group.length !== 4 || group.some((bar, index) => bar.time !== time + index * 3600)) return []
      return [{ time, open: group[0].open, high: Math.max(...group.map(bar => bar.high)), low: Math.min(...group.map(bar => bar.low)), close: group[3].close, volume: 0 }]
    })
  }
  const bucket = timeframe === '1W' ? 7 * 86400 : 86400
  const grouped = new Map<number, DemoFxBar>()
  for (const bar of bars) {
    const key = Math.floor(bar.time / bucket) * bucket
    const prior = grouped.get(key)
    if (!prior) grouped.set(key, { ...bar, time: key })
    else { prior.high = Math.max(prior.high, bar.high); prior.low = Math.min(prior.low, bar.low); prior.close = bar.close }
  }
  return [...grouped.values()]
}

// Mirrors the shared market session windows using New York local hours.
// Date parts come from Intl so the same UTC hour moves with DST.
export function demoFxSessions(symbol: string, from: number, to: number): ForexSession[] {
  return demoSessionsFromBars(hourlyCache[symbol], from, to)
}

export function demoSessionsFromBars(bars: DemoFxBar[], from: number, to: number): ForexSession[] {
  const groups = new Map<string, ForexSession>()
  for (const bar of bars) {
    if (bar.time < from - 86400 || bar.time > to + 86400) continue
    const { hour, day } = nyParts(bar.time)
    const name = hour >= 20 ? 'asia' : hour >= 2 && hour < 5 ? 'london' : hour >= 7 && hour < 10 ? 'new_york' : null
    if (!name) continue
    const key = `${day}-${name}`
    const previous = groups.get(key)
    if (previous) { previous.end = bar.time + 3600; previous.high = Math.max(previous.high, bar.high); previous.low = Math.min(previous.low, bar.low) }
    else groups.set(key, { name, start: bar.time, end: bar.time + 3600, high: bar.high, low: bar.low })
  }
  const sessions = [...groups.values()].filter(session => session.end >= from && session.start <= to)
  for (const session of sessions) if (session.name === 'asia') {
    session.asian_high = session.high
    session.asian_low = session.low
  }
  return sessions
}

export function demoDxy() {
  return Array.from({ length: 80 }, (_, i) => ({ time: Date.UTC(2026, 7, i + 1) / 1000, close: 102.5 - i * 0.015 + Math.sin(i * 0.4) * 0.3 }))
}
