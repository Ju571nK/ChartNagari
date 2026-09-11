import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'

export const pages = ['chart', 'analysis', 'history', 'calendar', 'backtest', 'performance', 'paper', 'my-trades', 'execution', 'symbols', 'rules', 'alert', 'price-alerts', 'report', 'status', 'settings'] as const
export type Page = typeof pages[number]
export const timeframes = ['1W', '1D', '4H', '1H'] as const
export type Timeframe = typeof timeframes[number]
type Selection = { page: Page; symbol: string; timeframe: Timeframe }

export function readSelection(): Selection {
  const params = new URLSearchParams(window.location.search)
  const page = params.get('view') as Page
  const timeframe = params.get('tf') as Timeframe
  return {
    page: pages.includes(page) ? page : 'chart',
    symbol: (params.get('symbol') || '').trim().toUpperCase(),
    timeframe: timeframes.includes(timeframe) ? timeframe : '1H',
  }
}

type WorkspaceValue = Selection & {
  navigate: (page: Page) => void
  setSymbol: (symbol: string) => void
  setTimeframe: (timeframe: Timeframe) => void
}
const WorkspaceContext = createContext<WorkspaceValue | null>(null)

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const [selection, setSelection] = useState(readSelection)
  useEffect(() => {
    const restore = () => setSelection(readSelection())
    window.addEventListener('popstate', restore)
    return () => window.removeEventListener('popstate', restore)
  }, [])
  const update = useCallback((patch: Partial<Selection>, push = false) => {
    // Read URL at interaction time so successive changes cannot overwrite one another.
    const next = { ...readSelection(), ...patch }
    const url = new URL(window.location.href)
    url.searchParams.set('view', next.page)
    if (next.symbol) url.searchParams.set('symbol', next.symbol)
    else url.searchParams.delete('symbol')
    url.searchParams.set('tf', next.timeframe)
    if (push) window.history.pushState(null, '', url)
    else window.history.replaceState(null, '', url)
    setSelection(next)
  }, [])
  const navigate = useCallback((page: Page) => update({ page }, true), [update])
  const setSymbol = useCallback((symbol: string) => update({ symbol: symbol.trim().toUpperCase() }), [update])
  const setTimeframe = useCallback((timeframe: Timeframe) => update({ timeframe }), [update])
  return <WorkspaceContext.Provider value={{ ...selection, navigate, setSymbol, setTimeframe }}>{children}</WorkspaceContext.Provider>
}

export function useWorkspace() {
  const value = useContext(WorkspaceContext)
  if (!value) throw new Error('WorkspaceProvider is required')
  return value
}
