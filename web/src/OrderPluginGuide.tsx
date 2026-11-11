import { useId } from 'react';
import { useTranslation } from 'react-i18next';

const docsRoot = 'https://github.com/Ju571nK/ChartNagari/blob/main/docs/order-plugins/';

export default function OrderPluginGuide() {
  const { t, i18n } = useTranslation();
  const headingId = useId();
  const language = (i18n.resolvedLanguage || i18n.language).split('-')[0];
  const quickStart = language === 'ko' ? 'ko.md' : language === 'ja' ? 'ja.md' : 'en.md';

  return (
    <section className="order-plugin-guide" aria-labelledby={headingId}>
      <h2 id={headingId}>{t('pluginGuide.title')}</h2>
      <p>{t('pluginGuide.scope')}</p>
      <details>
        <summary>{t('pluginGuide.summary')}</summary>
        <div className="order-plugin-guide-content">
          <h3>{t('pluginGuide.usersTitle')}</h3>
          <p>{t('pluginGuide.users')}</p>
          <p>{t('pluginGuide.legacy')}</p>
          <h3>{t('pluginGuide.developersTitle')}</h3>
          <ol>
            <li>{t('pluginGuide.stepTemplate')}</li>
            <li>{t('pluginGuide.stepInspect')}</li>
            <li>{t('pluginGuide.stepImplement')}</li>
            <li>{t('pluginGuide.stepTest')}</li>
          </ol>
          <p>{t('pluginGuide.safety')}</p>
          <p>{t('pluginGuide.remote')}</p>
          <nav className="order-plugin-guide-links" aria-label={t('pluginGuide.linksLabel')}>
            <a href={docsRoot + quickStart} target="_blank" rel="noopener noreferrer">{t('pluginGuide.startLink')}</a>
            <a href={docsRoot + 'protocol.md'} target="_blank" rel="noopener noreferrer">{t('pluginGuide.referenceLink')}</a>
          </nav>
        </div>
      </details>
    </section>
  );
}
