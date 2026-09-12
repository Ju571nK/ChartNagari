import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { CalendarTab, ContextSettings } from './App'
import i18n from './i18n'

beforeEach(async () => { await i18n.changeLanguage('en') })
afterEach(() => { cleanup(); vi.restoreAllMocks() })

async function openSettings(title: string) {
  const summary = await screen.findByText(title, {selector:'summary'})
  const details = summary.closest('details')!
  details.open = true
  fireEvent(details, new Event('toggle'))
}

it('offers focused calendar settings on empty results and saves only edited fields', async () => {
  const fetcher = vi.spyOn(globalThis,'fetch').mockImplementation(async input => new Response(JSON.stringify(String(input).startsWith('/api/calendar?') ? [] : {FMP_API_KEY:'__configured__',CALENDAR_ALERT_WINDOW:'30'})))
  render(<CalendarTab />)
  await screen.findByText('No calendar data available.')
  expect(screen.queryByText(/A paid API key is required/)).toBeNull()
  expect(fetcher.mock.calls.some(([url])=>url==='/api/settings/config')).toBe(false)
  await openSettings('Configure calendar')
  const key = await screen.findByLabelText('FMP API Key')
  expect(key).toHaveValue('')
  expect(key).toHaveAttribute('type','password')
  expect(screen.queryByLabelText('Server Port')).toBeNull()
  expect(screen.queryByLabelText('Tiingo API Key')).toBeNull()
  fireEvent.change(screen.getByLabelText('Advance alert window (minutes)'),{target:{value:'45'}})
  fireEvent.click(screen.getByRole('button',{name:'Save'}))
  await waitFor(()=>expect(fetcher.mock.calls.some(([,init])=>init?.method==='PUT')).toBe(true))
  const request=fetcher.mock.calls.find(([,init])=>init?.method==='PUT')!
  expect(JSON.parse(String(request[1]?.body))).toEqual({CALENDAR_ALERT_WINDOW:'45'})
  await screen.findByText(/Saved. See the startup comparison/, {selector:'.save-success'})
})

it('keeps setup reachable on calendar errors and does not confuse errors with empty data', async () => {
  vi.spyOn(globalThis,'fetch').mockResolvedValue(new Response('',{status:500}))
  render(<CalendarTab />)
  await screen.findByRole('alert')
  expect(screen.queryByText('No calendar data available.')).toBeNull()
  expect(screen.getByText('Configure calendar')).toBeInTheDocument()
})

it('opens an actionable AI settings form without making a mutation', async () => {
  const fetcher=vi.spyOn(globalThis,'fetch').mockImplementation(async input=>String(input).includes('/ollama/status') ? new Response('',{status:404}) : new Response('{}'))
  render(<ContextSettings kind="ai" uiMode="beginner" onSetUiMode={vi.fn()} />)
  await openSettings('Configure AI / LLM')
  await screen.findByLabelText('LLM Provider')
  expect(screen.getByLabelText('OpenAI API Key')).toHaveAttribute('type','password')
  expect(fetcher.mock.calls.every(([,init])=>!init?.method||init.method==='GET')).toBe(true)
})

it('aborts calendar requests on unmount', () => {
  let signal: AbortSignal | undefined
  vi.spyOn(globalThis,'fetch').mockImplementation((_url,init)=>{signal=init?.signal as AbortSignal;return new Promise(()=>{})})
  const view=render(<CalendarTab />)
  view.unmount()
  expect(signal?.aborted).toBe(true)
})

it('retains calendar edits after failed saves and calendar refreshes', async () => {
  vi.spyOn(globalThis,'fetch').mockImplementation(async(input,init)=>init?.method==='PUT' ? new Response('Authorization required',{status:401}) : new Response(JSON.stringify(String(input).startsWith('/api/calendar?') ? [] : {})))
  render(<CalendarTab />)
  await screen.findByText('No calendar data available.')
  await openSettings('Configure calendar')
  fireEvent.change(await screen.findByLabelText('FMP API Key'),{target:{value:'fixture-only'}})
  fireEvent.click(screen.getByRole('button',{name:'Save'}))
  expect(await screen.findByRole('alert')).toHaveTextContent('Authorization required')
  expect(screen.getByLabelText('FMP API Key')).toHaveValue('fixture-only')
  fireEvent.click(screen.getByRole('button',{name:'Refresh'}))
  await screen.findByText('No calendar data available.')
  expect(screen.getByLabelText('FMP API Key')).toHaveValue('fixture-only')
})
