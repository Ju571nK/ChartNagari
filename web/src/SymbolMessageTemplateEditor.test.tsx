import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { SymbolMessageTemplateEditor } from './SymbolMessageTemplateEditor'
import i18n from './i18n'

const reply = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
function deferredResponse() {
  let resolve!: (response: Response) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<Response>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
beforeEach(async () => { await i18n.changeLanguage('en') })
afterEach(() => vi.restoreAllMocks())

it('previews drafts without saving, then saves and resets each direction explicitly', async () => {
  let stored = { symbol: 'EURUSD', long: 'Old long', short: 'Old short' }
  const calls: Array<{ path: string; method: string; body?: Record<string, string> }> = []
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const path = String(input)
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) as Record<string, string> : undefined
    calls.push({ path, method, body })
    if (method === 'PUT') { stored = { symbol: 'EURUSD', long: body!.long, short: body!.short }; return reply(stored) }
    if (method === 'POST') return reply({ html: '<b>Core</b>\nCustom &lt;note&gt;', custom_included: true, sample: 'EURUSD · 1H · entry 1.10000' })
    return reply(stored)
  })
  render(<SymbolMessageTemplateEditor symbol="EURUSD" />)
  const input = await screen.findByRole('textbox', { name: 'LONG message text' }) as HTMLTextAreaElement
  await waitFor(() => expect(input.value).toBe('Old long'))
  fireEvent.change(input, { target: { value: 'Draft <note>' } })
  fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
  expect(await screen.findByRole('region', { name: 'Preview' })).toHaveTextContent('Custom <note>')
  expect(calls.filter(call => call.method === 'PUT')).toHaveLength(0)
  expect(calls.find(call => call.method === 'POST')?.body).toEqual({ direction: 'LONG', template: 'Draft <note>' })
  fireEvent.click(screen.getByRole('tab', { name: 'SHORT' }))
  expect((screen.getByRole('textbox', { name: 'SHORT message text' }) as HTMLTextAreaElement).value).toBe('Old short')
  fireEvent.click(screen.getByRole('button', { name: 'Reset direction to default' }))
  expect(calls.filter(call => call.method === 'PUT')).toHaveLength(0)
  fireEvent.click(screen.getByRole('button', { name: 'Save wording' }))
  await screen.findByText('Wording saved')
  expect(calls.filter(call => call.method === 'PUT')[0].body).toEqual({ long: 'Draft <note>', short: '' })
})

it('keeps the draft after a save error and rejects stale symbol responses', async () => {
  let oldGET: ((response: Response) => void) | undefined
  const puts: Array<{ path: string; body: Record<string, string> }> = []
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const path = String(input)
    if (init?.method === 'PUT') {
      puts.push({ path, body: JSON.parse(String(init.body)) as Record<string, string> })
      return reply({ error: 'save unavailable' }, 503)
    }
    if (path.endsWith('/EURUSD')) return new Promise(resolve => { oldGET = resolve })
    return reply({ symbol: 'USDJPY', long: 'Japan long', short: '' })
  })
  const view = render(<SymbolMessageTemplateEditor key="EURUSD" symbol="EURUSD" />)
  await waitFor(() => expect(oldGET).toBeDefined())
  view.rerender(<SymbolMessageTemplateEditor key="USDJPY" symbol="USDJPY" />)
  const input = await screen.findByRole('textbox', { name: 'LONG message text' }) as HTMLTextAreaElement
  await waitFor(() => expect(input.value).toBe('Japan long'))
  await act(async () => oldGET!(reply({ symbol: 'EURUSD', long: 'Old stale', short: '' })))
  expect(input.value).toBe('Japan long')
  fireEvent.change(input, { target: { value: 'New draft' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save wording' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('save unavailable')
  expect(input.value).toBe('New draft')
  expect(puts).toEqual([{ path: '/api/symbol-message-templates/USDJPY', body: { long: 'New draft', short: '' } }])
})

it('does not apply an old save response or send an old draft to a newly selected symbol', async () => {
  let finishOldSave: ((response: Response) => void) | undefined
  const puts: Array<{ path: string; body: Record<string, string> }> = []
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const path = String(input)
    if (init?.method === 'PUT') {
      puts.push({ path, body: JSON.parse(String(init.body)) as Record<string, string> })
      if (path.endsWith('/EURUSD')) return new Promise(resolve => { finishOldSave = resolve })
      return reply({ symbol: 'USDJPY', long: 'New yen', short: '' })
    }
    if (path.endsWith('/EURUSD')) return reply({ symbol: 'EURUSD', long: 'Old euro', short: '' })
    return reply({ symbol: 'USDJPY', long: 'Saved yen', short: '' })
  })
  const view = render(<SymbolMessageTemplateEditor symbol="EURUSD" />)
  let input = await screen.findByRole('textbox', { name: 'LONG message text' }) as HTMLTextAreaElement
  await waitFor(() => expect(input.value).toBe('Old euro'))
  fireEvent.change(input, { target: { value: 'Draft euro' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save wording' }))
  await waitFor(() => expect(finishOldSave).toBeDefined())
  view.rerender(<SymbolMessageTemplateEditor symbol="USDJPY" />)
  input = screen.getByRole('textbox', { name: 'LONG message text' }) as HTMLTextAreaElement
  await waitFor(() => expect(input.value).toBe('Saved yen'))
  await act(async () => finishOldSave!(reply({ symbol: 'EURUSD', long: 'Draft euro', short: '' })))
  expect(input.value).toBe('Saved yen')
  fireEvent.change(input, { target: { value: 'New yen' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save wording' }))
  await screen.findByText('Wording saved')
  expect(puts).toEqual([
    { path: '/api/symbol-message-templates/EURUSD', body: { long: 'Draft euro', short: '' } },
    { path: '/api/symbol-message-templates/USDJPY', body: { long: 'New yen', short: '' } },
  ])
})

it('ignores a failed preview after editing while a newer preview is busy', async () => {
  const oldPreview = deferredResponse()
  const currentPreview = deferredResponse()
  let previewCalls = 0
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (_input, init) => {
    if (init?.method === 'POST') return ++previewCalls === 1 ? oldPreview.promise : currentPreview.promise
    return reply({ symbol: 'EURUSD', long: '', short: '' })
  })
  render(<SymbolMessageTemplateEditor symbol="EURUSD" />)
  const input = await screen.findByRole('textbox', { name: 'LONG message text' })
  await waitFor(() => expect(screen.getByRole('button', { name: 'Preview' })).toBeEnabled())
  fireEvent.change(input, { target: { value: '{unknown}' } })
  fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
  expect(screen.getByRole('button', { name: 'Loading...' })).toBeDisabled()
  fireEvent.change(input, { target: { value: 'Corrected wording' } })
  fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
  expect(previewCalls).toBe(2)
  await act(async () => oldPreview.resolve(reply({ error: 'unknown placeholder {unknown}' }, 400)))
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Loading...' })).toBeDisabled()
  await act(async () => currentPreview.resolve(reply({ html: '<b>Current preview</b>', custom_included: true, sample: 'new sample' })))
  expect(screen.getByRole('button', { name: 'Preview' })).toBeEnabled()
  expect(screen.getByRole('region', { name: 'Preview' })).toHaveTextContent('Current preview')
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

it('ignores a failed preview after changing direction or saving corrected wording', async () => {
  const oldPreview = deferredResponse()
  const shortPreview = deferredResponse()
  const savePreview = deferredResponse()
  let previewCalls = 0
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (_input, init) => {
    if (init?.method === 'POST') return [oldPreview, shortPreview, savePreview][previewCalls++].promise
    if (init?.method === 'PUT') return reply({ symbol: 'EURUSD', long: '', short: 'Corrected short' })
    return reply({ symbol: 'EURUSD', long: '{unknown}', short: 'Old short' })
  })
  render(<SymbolMessageTemplateEditor symbol="EURUSD" />)
  await waitFor(() => expect(screen.getByRole('button', { name: 'Preview' })).toBeEnabled())
  fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
  fireEvent.click(screen.getByRole('tab', { name: 'SHORT' }))
  fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
  await act(async () => oldPreview.reject(new Error('old LONG network failure')))
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Loading...' })).toBeDisabled()
  fireEvent.change(screen.getByRole('textbox', { name: 'SHORT message text' }), { target: { value: 'Corrected short' } })
  fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
  fireEvent.click(screen.getByRole('button', { name: 'Save wording' }))
  await screen.findByText('Wording saved')
  await act(async () => shortPreview.reject(new Error('old SHORT network failure')))
  await act(async () => savePreview.resolve(reply({ error: 'obsolete validation error' }, 400)))
  expect(screen.getByRole('button', { name: 'Preview' })).toBeEnabled()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(screen.getByText('Wording saved')).toBeInTheDocument()
})

it('ignores a failed preview from a previous symbol while the new symbol preview is busy', async () => {
  const oldPreview = deferredResponse()
  const currentPreview = deferredResponse()
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const path = String(input)
    if (init?.method === 'POST') return path.includes('EURUSD') ? oldPreview.promise : currentPreview.promise
    return path.includes('EURUSD')
      ? reply({ symbol: 'EURUSD', long: 'Euro wording', short: '' })
      : reply({ symbol: 'USDJPY', long: 'Yen wording', short: '' })
  })
  const view = render(<SymbolMessageTemplateEditor symbol="EURUSD" />)
  await waitFor(() => expect(screen.getByRole('button', { name: 'Preview' })).toBeEnabled())
  fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
  view.rerender(<SymbolMessageTemplateEditor symbol="USDJPY" />)
  await waitFor(() => expect((screen.getByRole('textbox', { name: 'LONG message text' }) as HTMLTextAreaElement).value).toBe('Yen wording'))
  fireEvent.click(screen.getByRole('button', { name: 'Preview' }))
  await act(async () => oldPreview.reject(new Error('old symbol network failure')))
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Loading...' })).toBeDisabled()
  await act(async () => currentPreview.resolve(reply({ html: 'Yen preview', custom_included: true, sample: 'yen sample' })))
  expect(screen.getByRole('region', { name: 'Preview' })).toHaveTextContent('Yen preview')
  expect(screen.getByRole('button', { name: 'Preview' })).toBeEnabled()
})
