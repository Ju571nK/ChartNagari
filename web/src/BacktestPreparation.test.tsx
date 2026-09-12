import { render, screen, fireEvent } from '@testing-library/react'
import { beforeEach, expect, it, vi, afterEach } from 'vitest'
import { BacktestPreparation, type Readiness } from './BacktestPreparation'
import { BacktestTab } from './App'
import { WorkspaceProvider } from './Workspace'
import i18n from './i18n'

const ready: Readiness = {symbol:'SPCX',timeframe:'1H',bars:202,minimum_bars:202,from:1700000000,to:1700010000,ready:true}
beforeEach(async () => { await i18n.changeLanguage('en'); window.history.replaceState(null,'','/?view=backtest&symbol=SPCX&tf=1H') })
afterEach(() => vi.restoreAllMocks())

it.each([{...ready,bars:0,ready:false}, {...ready,bars:201,ready:false},ready])('gates execution for $bars bars', async data => {
  vi.spyOn(globalThis,'fetch').mockImplementation(async input => ({ok:true,json:async () => String(input).includes('/readiness?') ? data : []}) as Response)
  render(<WorkspaceProvider><BacktestTab uiMode="beginner" /></WorkspaceProvider>)
  await screen.findByText(new RegExp(`${data.bars} candles in full history`))
  const run = screen.getByRole('button',{name:i18n.t('run')})
  if(data.ready) expect(run).toBeEnabled()
  else expect(run).toBeDisabled()
  expect(screen.getByLabelText('Take-profit ATR multiplier')).toHaveValue(2)
  expect(screen.getByLabelText('Stop-loss ATR multiplier')).toHaveValue(1)
})
it('keeps execution disabled when preparation fails', async () => {
  vi.spyOn(globalThis,'fetch').mockImplementation(async input => ({ok:!String(input).includes('/readiness?'),status:503,json:async () => []}) as Response)
  render(<WorkspaceProvider><BacktestTab uiMode="beginner" /></WorkspaceProvider>)
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not check backtest data')
  expect(screen.getByRole('button',{name:i18n.t('run')})).toBeDisabled()
})
it.each(['en','ko','ja'])('explains ATR and historical limitations in %s', async lang => {
  await i18n.changeLanguage(lang)
  const { container } = render(<BacktestPreparation data={ready} symbol="SPCX" onRetry={() => {}} />)
  expect(container.textContent).toContain('ATR')
  expect(container.textContent).not.toContain('backtestPrep.')
  expect(screen.getByRole('status')).toHaveTextContent(i18n.t('backtestPrep.ready'))
})

it.each([false,true])('distinguishes a failed run (%s) from zero matching trades', async fail => {
  vi.spyOn(globalThis,'fetch').mockImplementation(async (input, options) => ({
    ok: options?.method === 'POST' ? !fail : true, status: fail ? 500 : 200,
    json:async () => String(input).includes('/readiness?') ? ready : options?.method === 'POST' ? {symbol:'SPCX',timeframe:'1H',bars:202,trades:0} : [],
  }) as Response)
  render(<WorkspaceProvider><BacktestTab uiMode="beginner" /></WorkspaceProvider>)
  await screen.findByText(/202 candles in full history/)
  fireEvent.click(screen.getByRole('button',{name:i18n.t('run')}))
  if (fail) expect(await screen.findByRole('alert')).toHaveTextContent('Backtest failed')
  else {
    expect(await screen.findByText(/No simulated trades matched/)).toBeInTheDocument()
    expect(screen.queryByText('0.0%')).not.toBeInTheDocument()
  }
})
