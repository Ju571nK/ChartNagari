import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import { beforeEach, afterEach, expect, it, vi } from 'vitest'
import { SymbolsTab } from './App'
import { WorkspaceProvider } from './Workspace'
import i18n from './i18n'

beforeEach(async () => { await i18n.changeLanguage('en'); window.history.replaceState(null,'','/?view=symbols&symbol=SPCX') })
afterEach(() => vi.restoreAllMocks())
const response = (data:unknown, ok=true) => ({ok,status:ok?200:502,json:async()=>data}) as Response
async function setup(validation:unknown, ok=true, existing=false) {
  const fetchMock = vi.spyOn(globalThis,'fetch').mockImplementation(async input => {
    const path = String(input)
    if(path.includes('/validate?')) return response(validation,ok)
    if(path.endsWith('/symbols') && existing) return response([{symbol:'SPCX',type:'stock',exchange:'nasdaq',enabled:true}])
    return response([])
  })
  render(<WorkspaceProvider><SymbolsTab /></WorkspaceProvider>)
  const input = await screen.findByPlaceholderText(i18n.t('symbol_placeholder_nvda'))
  fireEvent.change(input,{target:{value:'SPCX'}})
  expect(screen.getByRole('button',{name:i18n.t('add')})).toBeDisabled()
  return fetchMock
}
it('requires verification and distinguishes registration from data readiness', async () => {
  const fetchMock = await setup({found:true,type:'stock',exchange:'nasdaq',name:'SPCX name'})
  await screen.findByText(/SPCX name/,{}, {timeout:2000})
  fireEvent.click(screen.getByRole('button',{name:i18n.t('add')}))
  expect(await screen.findByText(/SPCX registered/)).toBeInTheDocument()
  expect(await screen.findByText(/No stored candles/)).toBeInTheDocument()
  const post = fetchMock.mock.calls.find(([,options])=>options?.method==='POST')
  expect(JSON.parse(String(post?.[1]?.body))).toEqual({symbol:'SPCX',type:'stock',exchange:'nasdaq'})
})
it.each([true,false])('blocks unsupported or unavailable verification (provider OK=%s)', async ok => {
  await setup({found:false},ok)
  await screen.findByText(ok?/Instrument not confirmed/:/validation provider could not be reached/,{}, {timeout:2000})
  expect(screen.getByRole('button',{name:i18n.t('add')})).toBeDisabled()
})
it('blocks duplicate registration without hiding the form', async () => {
  const fetchMock = await setup({found:true,type:'stock',exchange:'nasdaq',name:'SPCX name'},true,true)
  await screen.findByText(/SPCX name/,{}, {timeout:2000})
  fireEvent.click(screen.getByRole('button',{name:i18n.t('add')}))
  expect(await screen.findByRole('alert')).toHaveTextContent('already registered')
  expect(fetchMock.mock.calls.some(([,options])=>options?.method==='POST')).toBe(false)
  expect(screen.getByPlaceholderText(i18n.t('symbol_placeholder_nvda'))).toBeInTheDocument()
})
it('ignores an old verification response after the symbol changes', async () => {
  let resolveOld: ((response:Response)=>void) | undefined
  vi.spyOn(globalThis,'fetch').mockImplementation(async input => {
    const path = String(input)
    if(path.includes('symbol=SPCX')) return new Promise(resolve=>{resolveOld=resolve})
    if(path.includes('symbol=TSLA')) return response({found:true,type:'stock',exchange:'nasdaq',name:'Tesla'})
    return response([])
  })
  render(<WorkspaceProvider><SymbolsTab /></WorkspaceProvider>)
  const input = await screen.findByPlaceholderText(i18n.t('symbol_placeholder_nvda'))
  fireEvent.change(input,{target:{value:'SPCX'}})
  await waitFor(()=>expect(resolveOld).toBeDefined(),{timeout:2000})
  fireEvent.change(input,{target:{value:'TSLA'}})
  await act(async()=>resolveOld!(response({found:true,type:'stock',exchange:'wrong',name:'Old SPCX'})))
  expect(screen.queryByText(/Old SPCX/)).not.toBeInTheDocument()
  expect(await screen.findByText(/Tesla/,{}, {timeout:2000})).toBeInTheDocument()
})
