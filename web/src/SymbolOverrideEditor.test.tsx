import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import i18n from './i18n'
import { SymbolOverrideEditor } from './SymbolOverrideEditor'

const profile = {
  name: 'large_cap_stock',
  allowed_methodologies: [],
  blocked_methodologies: [],
  allowed_rules: ['ict_order_block'],
  alert_limit_per_day: 2,
  cooldown_hours: 8,
  score_threshold: 10,
}

const wrap = (ui: React.ReactElement) => (
  <I18nextProvider i18n={i18n}>{ui}</I18nextProvider>
)

beforeEach(() => {
  vi.useFakeTimers()
  vi.spyOn(globalThis, 'fetch').mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({
      symbol: 'TSLA',
      score_threshold: { value: 10, source: 'profile' },
      cooldown_hours: { value: 8, source: 'profile' },
      alert_limit_per_day: { value: 2, source: 'profile' },
      timeframes: { value: [], source: 'profile' },
      allowed_rules: { value: ['ict_order_block'], source: 'profile' },
    }),
  } as Response)
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('SymbolOverrideEditor', () => {
  it('renders profile default values when no override exists', async () => {
    render(wrap(<SymbolOverrideEditor symbol="TSLA" profile={profile} />))
    await vi.runAllTimersAsync()
    expect(screen.getByText(/Score threshold|점수 임계값|スコア閾値/)).toBeDefined()
    const slider = screen.getByLabelText(/Score threshold|점수 임계값|スコア閾値/) as HTMLInputElement
    expect(Number(slider.value)).toBe(10)
  })

  it('debounces PUT 500ms after slider change', async () => {
    render(wrap(<SymbolOverrideEditor symbol="TSLA" profile={profile} />))
    await vi.runAllTimersAsync()

    const slider = screen.getByLabelText(/Score threshold|점수 임계값|スコア閾値/) as HTMLInputElement
    fireEvent.change(slider, { target: { value: '14' } })

    // Initial GET only.
    expect(globalThis.fetch).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(499)
    expect(globalThis.fetch).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(2)
    await vi.runAllTimersAsync()

    expect(globalThis.fetch).toHaveBeenCalledTimes(2)
    const putCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[1]
    expect(putCall[0]).toBe('/api/symbol-overrides/TSLA')
    const init = putCall[1] as RequestInit
    expect(init.method).toBe('PUT')
    const body = JSON.parse(init.body as string)
    expect(body.score_threshold).toBe(14)
  })

  it('reset button sends null for that field via PUT', async () => {
    render(wrap(<SymbolOverrideEditor symbol="TSLA" profile={profile} />))
    await vi.runAllTimersAsync()

    const slider = screen.getByLabelText(/Score threshold|점수 임계값|スコア閾値/) as HTMLInputElement
    fireEvent.change(slider, { target: { value: '14' } })
    vi.advanceTimersByTime(501)
    await vi.runAllTimersAsync()

    const resetBtn = screen.getByTestId('reset-score_threshold')
    fireEvent.click(resetBtn)
    vi.advanceTimersByTime(501)
    await vi.runAllTimersAsync()

    const lastCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.at(-1)!
    const init = lastCall[1] as RequestInit
    const body = JSON.parse(init.body as string)
    expect(body.score_threshold).toBeNull()
  })

  it('reset-all button sends DELETE', async () => {
    render(wrap(<SymbolOverrideEditor symbol="TSLA" profile={profile} />))
    await vi.runAllTimersAsync()

    const resetAll = screen.getByTestId('reset-all')
    fireEvent.click(resetAll)
    await vi.runAllTimersAsync()

    const lastCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.at(-1)!
    const init = lastCall[1] as RequestInit
    expect(init.method).toBe('DELETE')
    expect(lastCall[0]).toBe('/api/symbol-overrides/TSLA')
  })
})
