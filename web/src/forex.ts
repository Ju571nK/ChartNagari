export type ForexProvider = 'auto' | 'yahoo' | 'oanda'
export type SessionName = 'asia' | 'london' | 'new_york'
export interface ForexSession {
  name: SessionName
  start: number
  end: number
  high: number
  low: number
  asian_high?: number
  asian_low?: number
}
export interface StrengthCurrency {
  currency: string
  value: number | null
  coverage: 'ok' | 'low'
  pairs: number
}
export interface DxyData {
  state?: 'disabled'
  bars: { time: number; close: number }[]
  correlation_20: number | null
  correlation_60: number | null
}
export interface ForexStatus {
  provider: 'yahoo' | 'oanda'
  configured_provider: ForexProvider
  volume_quality: 'none' | 'tick'
  state: 'ready' | 'degraded' | 'error' | 'rebackfilling'
  message?: string
  provider_changed?: boolean
}

export const FX_MAJORS = ['EURUSD', 'GBPUSD', 'USDJPY', 'USDCHF', 'AUDUSD', 'USDCAD', 'NZDUSD'] as const
export const FX_ALL_28 = [
  'EURUSD', 'GBPUSD', 'USDJPY', 'USDCHF', 'AUDUSD', 'USDCAD', 'NZDUSD',
  'EURGBP', 'EURJPY', 'EURCHF', 'EURAUD', 'EURCAD', 'EURNZD',
  'GBPJPY', 'GBPCHF', 'GBPAUD', 'GBPCAD', 'GBPNZD',
  'AUDJPY', 'AUDCHF', 'AUDCAD', 'AUDNZD',
  'NZDJPY', 'NZDCHF', 'NZDCAD', 'CADJPY', 'CADCHF', 'CHFJPY',
] as const
export const FX_PRESETS = {
  majors: [...FX_MAJORS],
  majorsGold: [...FX_MAJORS, 'XAUUSD'],
  all28: [...FX_ALL_28],
} as const
const currencies = new Set(['USD', 'EUR', 'GBP', 'JPY', 'CHF', 'CAD', 'AUD', 'NZD'])
export function isForexSymbol(symbol: string): boolean {
  if (!/^[A-Z]{6}$/.test(symbol)) return false
  const base = symbol.slice(0, 3), quote = symbol.slice(3)
  return (currencies.has(base) && currencies.has(quote) && base !== quote) ||
    ((base === 'XAU' || base === 'XAG') && quote === 'USD')
}
export function isMetal(symbol: string): boolean { return symbol === 'XAUUSD' || symbol === 'XAGUSD' }
export function isUSDPair(symbol: string): boolean { return isForexSymbol(symbol) && (symbol.slice(0, 3) === 'USD' || symbol.slice(3) === 'USD') }
export function instrument(symbol: string) {
  const pip = symbol === 'XAUUSD' ? 0.1 : symbol.endsWith('JPY') || symbol === 'XAGUSD' ? 0.01 : 0.0001
  return { pip, precision: symbol === 'XAUUSD' ? 2 : symbol === 'XAGUSD' ? 3 : symbol.endsWith('JPY') ? 3 : 5 }
}
export function formatPrice(symbol: string, value: number): string { return value.toFixed(isForexSymbol(symbol) ? instrument(symbol).precision : 2) }
export function formatPips(value: number): string { return `${value >= 0 ? '+' : ''}${value.toFixed(1)}` }
export function pipDistance(symbol: string, from: number, to: number): number { return (to - from) / instrument(symbol).pip }
export function visibleSession(timeframe: string): boolean { return timeframe !== '1D' && timeframe !== '1W' }

export async function forexGet<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(`/api/forex/${path}`, { signal })
  if (!response.ok) throw new Error(`HTTP ${response.status}: ${response.statusText}`)
  return response.json() as Promise<T>
}
