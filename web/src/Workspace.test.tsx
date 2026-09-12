import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import './i18n'
import { WorkspaceProvider, readSelection, useWorkspace } from './Workspace'
import { WorkspaceNav } from './WorkspaceNav'

function Probe() {
  const state = useWorkspace()
  return <>
    <output>{state.page}/{state.symbol}/{state.timeframe}</output>
    <button onClick={() => { state.setSymbol('BTCUSDT'); state.setTimeframe('4H'); }}>Select</button>
  </>
}

describe('workspace navigation', () => {
  beforeEach(() => window.history.replaceState(null, '', '/'))

  it('normalizes direct links and rejects invalid pages and timeframes', () => {
    window.history.replaceState(null, '', '/?view=missing&symbol=spy&tf=invalid')
    expect(readSelection()).toEqual({ page: 'chart', symbol: 'SPY', timeframe: '1H' })
  })

  it('preserves selection when navigating and when remounted after a refresh', () => {
    const view = render(<WorkspaceProvider><WorkspaceNav /><Probe /></WorkspaceProvider>)
    fireEvent.click(screen.getByRole('button', { name: 'Select' }))
    fireEvent.click(screen.getByRole('button', { name: 'Backtest' }))
    expect(screen.getByText('backtest/BTCUSDT/4H')).toBeInTheDocument()
    expect(window.location.search).toContain('symbol=BTCUSDT')
    view.unmount()
    render(<WorkspaceProvider><Probe /></WorkspaceProvider>)
    expect(screen.getByText('backtest/BTCUSDT/4H')).toBeInTheDocument()
  })

  it('restores browser history and exposes all 16 destinations including price alerts', () => {
    render(<WorkspaceProvider><WorkspaceNav /><Probe /></WorkspaceProvider>)
    expect(screen.getByRole('navigation', { name: 'Workspace navigation' }).querySelectorAll('button')).toHaveLength(16)
    fireEvent.click(screen.getByRole('button', { name: 'Price Alerts' }))
    expect(readSelection().page).toBe('price-alerts')
    act(() => {
      window.history.replaceState(null, '', '/?view=analysis&symbol=SPY&tf=1D')
      window.dispatchEvent(new PopStateEvent('popstate'))
    })
    expect(screen.getByText('analysis/SPY/1D')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Analysis' })).toHaveAttribute('aria-current', 'page')
  })
})
