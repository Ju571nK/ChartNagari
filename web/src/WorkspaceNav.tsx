import { useTranslation } from 'react-i18next'
import { useWorkspace, type Page } from './Workspace'
import { useUXCopy } from './uxCopy'

const groups: { key: string; pages: Page[] }[] = [
  { key: 'explore', pages: ['chart', 'analysis', 'history', 'calendar'] },
  { key: 'validate', pages: ['backtest', 'performance'] },
  { key: 'trade', pages: ['paper', 'my-trades', 'execution'] },
  { key: 'configure', pages: ['symbols', 'rules', 'alert', 'price-alerts', 'report', 'status', 'settings'] },
]
export const pageKeys: Record<Page, string> = {
  chart: 'chart', analysis: 'analysis', history: 'history', calendar: 'calendar',
  backtest: 'backtest', performance: 'performance', paper: 'paper', 'my-trades': 'my_trades.title',
  execution: 'execution', symbols: 'symbols', rules: 'rules', alert: 'alert',
  'price-alerts': 'price_alerts', report: 'report', status: 'status', settings: 'settings',
}

export function WorkspaceNav({ mode = 'expert' }: { mode?: 'beginner' | 'expert' }) {
  const { t } = useTranslation()
  const ux = useUXCopy()
  const { page, navigate } = useWorkspace()
  const primary: Page[] = ['chart', 'symbols', 'alert', 'settings']
  const prefetchPage = (item: Page) => {
    const loaders: Partial<Record<Page, () => Promise<unknown>>> = {
      analysis: () => import('./AnalysisTab'),
      execution: () => import('./ExecutionTab'),
      'my-trades': () => import('./MyTradesTab'),
    }
    loaders[item]?.().catch(() => {})
  }
  return <>
    <div className="mobile-navigation">
      <nav className="mobile-primary" aria-label={ux.primary}>{primary.map(item => <button key={item} aria-current={page === item || (item === 'alert' && page === 'price-alerts') ? 'page' : undefined} onClick={() => navigate(item)}>{item === 'alert' ? ux.alerts : t(pageKeys[item])}</button>)}</nav>
      <details><summary>{ux.more}{!primary.includes(page) ? ` · ${t(pageKeys[page])}` : ''}</summary>
      <label>{t('workspace.navigation')}<select value={page} onChange={event => navigate(event.target.value as Page)}>
        {groups.map(group => <optgroup key={group.key} label={t(`workspace.${group.key}`)}>
          {group.pages.map(item => <option key={item} value={item}>{t(pageKeys[item])}</option>)}
        </optgroup>)}
      </select></label></details>
    </div>
    <nav className="workspace-nav" aria-label={t('workspace.navigation')}>
    {mode === 'beginner' && <section className="nav-group"><h2>{ux.primary}</h2>{primary.map(item => <button key={item} className={`nav-link${page === item ? ' active' : ''}`} aria-current={page === item ? 'page' : undefined} onClick={() => navigate(item)}>{item === 'alert' ? ux.alerts : t(pageKeys[item])}</button>)}</section>}
    <details className="desktop-more" open={mode === 'expert' || !primary.includes(page)}><summary>{ux.more}</summary>
    {groups.map(group => <section className="nav-group" key={group.key}>
      <h2>{t(`workspace.${group.key}`)}</h2>
      <div className="nav-items">{group.pages.filter(item => mode === 'expert' || !primary.includes(item)).map((item, index) => <button
        type="button" key={item} className={`nav-link${page === item ? ' active' : ''}`}
        aria-current={page === item ? 'page' : undefined} onClick={() => navigate(item)}
        onMouseEnter={() => prefetchPage(item)} onFocus={() => prefetchPage(item)}
      ><span className="nav-index" aria-hidden="true">{String(index + 1).padStart(2, '0')}</span>{t(pageKeys[item])}</button>)}</div>
    </section>)}</details>
  </nav></>
}
