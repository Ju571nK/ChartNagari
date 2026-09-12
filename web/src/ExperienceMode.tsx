import { useTranslation } from 'react-i18next'

export type UIMode = 'beginner' | 'expert'
export const UI_MODE_KEY = 'chartnagari_ui_mode'
export function loadUIMode(): UIMode {
  return localStorage.getItem(UI_MODE_KEY) === 'expert' ? 'expert' : 'beginner'
}

export function ExperienceMode({ mode, onChange }: { mode: UIMode; onChange: (mode: UIMode) => void }) {
  const { t } = useTranslation()
  return <label className="language-control">{t('experience.label')}
    <select value={mode} onChange={event => onChange(event.target.value as UIMode)}>
      <option value="beginner">{t('mode_beginner')}</option>
      <option value="expert">{t('mode_expert')}</option>
    </select>
  </label>
}

export function ChartGuide({ mode }: { mode: UIMode }) {
  const { t } = useTranslation()
  return <details className="chart-guide">
    <summary tabIndex={0}>{t('experience.help')}</summary>
    <p>{t(mode === 'beginner' ? 'experience.beginnerHelp' : 'experience.expertHelp')}</p>
    <dl>
      {['timeframe', 'signal', 'ict', 'smc', 'fvg', 'ob', 'wyckoff', 'ta', 'vix'].map(key => <div key={key}>
        <dt>{t(`experience.terms.${key}.name`)}</dt><dd>{t(`experience.terms.${key}.help`)}</dd>
      </div>)}
    </dl>
  </details>
}
