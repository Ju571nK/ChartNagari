export interface PatternBar { time: number; open: number; high: number; low: number; close: number }
export interface ChartPattern {
  kind: 'fvg' | 'ob'; direction: 'bull' | 'bear'; from: number; to: number; low: number; high: number
}

// Visual formation candidates, NOT the engine's retest/entry signals. Only use
// the selected chart's loaded candles. Remove fully filled gaps and OBs closed
// through after formation. Keep the latest five of each kind to avoid clutter.
export function chartPatterns(bars: PatternBar[]): ChartPattern[] {
  const patterns: ChartPattern[] = []
  const add = (kind: ChartPattern['kind'], direction: ChartPattern['direction'], i: number, low: number, high: number) => {
    if (!(high > low)) return
    const invalid = bars.slice(i + 3).some(b => kind === 'fvg'
      ? direction === 'bull' ? b.low <= low : b.high >= high
      : direction === 'bull' ? b.close < low : b.close > high)
    if (!invalid) patterns.push({ kind, direction, from: bars[i].time, to: bars[bars.length - 1].time, low, high })
  }
  for (let i = 0; i + 2 < bars.length; i++) {
    const [a, b, c] = bars.slice(i, i + 3)
    if (a.high < c.low) add('fvg', 'bull', i, a.high, c.low)
    if (a.low > c.high) add('fvg', 'bear', i, c.high, a.low)
    // Local true-range average at formation (up to 14 bars), never future ATR.
    const start = Math.max(0, i - 13)
    let tr = 0
    for (let j = start; j <= i; j++) {
      const prev = bars[Math.max(0, j - 1)].close
      tr += Math.max(bars[j].high - bars[j].low, Math.abs(bars[j].high - prev), Math.abs(bars[j].low - prev))
    }
    const atr = tr / (i - start + 1)
    if (Math.abs(b.close - b.open) + Math.abs(c.close - c.open) < 1.5 * atr) continue
    if (a.close < a.open && b.close > b.open && c.close > a.open) add('ob', 'bull', i, a.low, a.high)
    if (a.close > a.open && b.close < b.open && c.close < a.open) add('ob', 'bear', i, a.low, a.high)
  }
  return ['fvg', 'ob'].flatMap(kind => patterns.filter(p => p.kind === kind).slice(-5))
}
