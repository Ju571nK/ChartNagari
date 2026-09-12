import { fireEvent, render, screen, within, cleanup } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { WorkspaceProvider } from './Workspace'
import { WorkspaceNav } from './WorkspaceNav'
import { AlertHub, SettingsTab } from './App'
import { readableRule, signalRange } from './uxCopy'
import i18n from './i18n'

beforeEach(async () => { await i18n.changeLanguage('en'); window.history.replaceState(null, '', '/?view=chart') })
afterEach(() => { cleanup(); vi.restoreAllMocks() })

it('keeps beginner navigation short without removing expert destinations', () => {
  render(<WorkspaceProvider><WorkspaceNav mode="beginner" /></WorkspaceProvider>)
  const desktop = screen.getByRole('navigation', { name: 'Workspace navigation' })
  expect(desktop.querySelector(':scope > section')?.querySelectorAll('button')).toHaveLength(4)
  expect(desktop.querySelector('details')).not.toHaveAttribute('open')
  fireEvent.click(within(desktop).getByText('More'))
  expect(within(desktop).getByRole('button', { name: 'Execution' })).toBeInTheDocument()
  fireEvent.click(within(desktop).getByRole('button', { name: 'Backtest' }))
  expect(window.location.search).toContain('view=backtest')
})

it('classifies signal range with exclusive candle end and never truncates a rule', () => {
  expect(readableRule('bullish_engulfing')).toBe('Bullish Engulfing')
  expect(readableRule('bullish_engulfing', 'ko')).toBe('상승 장악형')
  expect(readableRule('hammer', 'ja')).toBe('ハンマー')
  expect(readableRule('ict_ote')).toBe('ICT OTE')
  expect(signalRange(100, [], '1H')).toBe('unavailable')
  expect(signalRange(99, [{time:100}], '1H')).toBe('archive')
  expect(signalRange(100, [{time:100}], '1H')).toBe('inRange')
  expect(signalRange(3699, [{time:100}], '1H')).toBe('inRange')
  expect(signalRange(3700, [{time:100}], '1H')).toBe('archive')
})

it('keeps operational settings out of General and retains edits across sections', async () => {
  vi.spyOn(globalThis,'fetch').mockImplementation(async () => new Response('{}'))
  render(<SettingsTab uiMode="beginner" onSetUiMode={vi.fn()} />)
  await screen.findByRole('tab', {name:'General'})
  expect(screen.queryByLabelText('Server Port')).toBeNull()
  expect(screen.getByLabelText('Language')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('tab', {name:'Advanced'}))
  fireEvent.change(screen.getByLabelText('Server Port'), {target:{value:'9000'}})
  fireEvent.click(screen.getByRole('tab', {name:'General'}))
  expect(screen.getByText('Unsaved changes')).toBeInTheDocument()
  expect(screen.getByRole('button',{name:'Save'})).toBeEnabled()
})

it('exposes settings load failures even on General', async () => {
  vi.spyOn(globalThis,'fetch').mockResolvedValue(new Response('',{status:500}))
  render(<SettingsTab uiMode="beginner" onSetUiMode={vi.fn()} />)
  expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load settings')
})

it('groups signal, price and delivery settings without sending mutations', async () => {
  const fetcher = vi.spyOn(globalThis,'fetch').mockImplementation(async () => new Response('{}'))
  render(<WorkspaceProvider><AlertHub uiMode="beginner" onSetUiMode={vi.fn()} /></WorkspaceProvider>)
  const tabs = screen.getByRole('group', {name:'Alerts'})
  expect(within(tabs).getByRole('button', {name:'Signal alerts'})).toHaveAttribute('aria-pressed','true')
  fireEvent.click(within(tabs).getByRole('button', {name:'Delivery channels'}))
  await screen.findByLabelText('Telegram Bot Token')
  expect(fetcher.mock.calls.every(([,init])=>!init?.method || init.method === 'GET')).toBe(true)
})
