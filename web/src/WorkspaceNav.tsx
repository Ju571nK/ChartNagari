import { useTranslation } from 'react-i18next'
import { useWorkspace, type Page } from './Workspace'

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

export function WorkspaceNav() {
  const { t } = useTranslation()
  const { page, navigate } = useWorkspace()
  return <>
    <label className="mobile-navigation">{t('workspace.navigation')}
      <select value={page} onChange={event => navigate(event.target.value as Page)}>
        {groups.map(group => <optgroup key={group.key} label={t(`workspace.${group.key}`)}>
          {group.pages.map(item => <option key={item} value={item}>{t(pageKeys[item])}</option>)}
        </optgroup>)}
      </select>
    </label>
    <nav className="workspace-nav" aria-label={t('workspace.navigation')}>
    {groups.map(group => <section className="nav-group" key={group.key}>
      <h2>{t(`workspace.${group.key}`)}</h2>
      <div className="nav-items">{group.pages.map((item, index) => <button
        type="button" key={item} className={`nav-link${page === item ? ' active' : ''}`}
        aria-current={page === item ? 'page' : undefined} onClick={() => navigate(item)}
      ><span className="nav-index" aria-hidden="true">{String(index + 1).padStart(2, '0')}</span>{t(pageKeys[item])}</button>)}</div>
    </section>)}
  </nav></>
}
