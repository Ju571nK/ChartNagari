import { expect, it } from 'vitest'
import { chartPatterns, type PatternBar } from './chartPatterns'

const bars: PatternBar[] = [
  { time: 1, open: 10, high: 11, low: 9, close: 9.5 },
  { time: 2, open: 9.5, high: 14, low: 9.5, close: 14 },
  { time: 3, open: 14, high: 16, low: 13, close: 15 },
]
it('detects FVG and OB candidates without any saved signals', () => {
  expect(chartPatterns(bars)).toEqual([
    { kind: 'fvg', direction: 'bull', from: 1, to: 3, low: 11, high: 13 },
    { kind: 'ob', direction: 'bull', from: 1, to: 3, low: 9, high: 11 },
  ])
})
it('removes filled gaps and closed-through order blocks', () => {
  const result = chartPatterns([...bars, { time: 4, open: 15, high: 15, low: 8, close: 8 }])
  expect(result.filter(p => p.from === 1)).toEqual([])
})
it('detects bearish formations symmetrically', () => {
  const mirrored = bars.map(b => ({ time: b.time, open: 30 - b.open, high: 30 - b.low, low: 30 - b.high, close: 30 - b.close }))
  expect(chartPatterns(mirrored).map(p => p.direction)).toEqual(['bear', 'bear'])
})
it('does not create ranges for insufficient or flat bars', () => {
  expect(chartPatterns([])).toEqual([])
  expect(chartPatterns(bars.slice(0, 2))).toEqual([])
  expect(chartPatterns([1, 2, 3, 4].map(time => ({time, open: 10, high: 11, low: 9, close: 10})))).toEqual([])
})
