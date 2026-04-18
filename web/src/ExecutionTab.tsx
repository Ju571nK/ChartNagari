import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import KillSwitch from './KillSwitch';
import PluginCard from './PluginCard';
import FeedbackTable from './FeedbackTable';

export type Plugin = {
  name: string;
  url: string;
  enabled: boolean;
  symbols: string[];
  min_score: number;
  direction_filter: '' | 'LONG' | 'SHORT';
  secret: string;
};

export type ExecutionConfig = {
  version: number;
  enabled: boolean;
  killed_at: string;
  plugins: Plugin[];
  max_dispatched: number;
  dedup_window: string;
  symbol_map: Record<string, Record<string, string>>;
};

export type PluginStat = {
  plugin_id: string;
  submitted: number;
  filled: number;
  rejected: number;
  last_failure_at?: number;
  last_failure_msg: string;
};

export type FeedbackRow = {
  plugin_id: string;
  signal_id: string;
  order_id: string;
  status: string;
  symbol: string;
  message: string;
  received_at: number;
};

export type FeedbackFilters = {
  plugin: string;
  status: string;
  symbol: string;
};

export default function ExecutionTab() {
  const { t } = useTranslation();
  const [config, setConfig] = useState<ExecutionConfig | null>(null);
  const [stats, setStats] = useState<PluginStat[]>([]);
  const [feedback, setFeedback] = useState<FeedbackRow[]>([]);
  const [filters, setFilters] = useState<FeedbackFilters>({ plugin: '', status: '', symbol: '' });
  const filtersRef = useRef<FeedbackFilters>(filters);
  useEffect(() => { filtersRef.current = filters; }, [filters]);

  const loadConfig = useCallback(async () => {
    const r = await fetch('/api/execution/config', { credentials: 'include' });
    if (r.ok) setConfig(await r.json());
  }, []);

  const loadStats = useCallback(async () => {
    const r = await fetch('/api/execution/plugins/stats?window=24h', { credentials: 'include' });
    if (r.ok) { const b = await r.json(); setStats(b.plugins ?? []); }
  }, []);

  const loadFeedback = useCallback(async () => {
    const f = filtersRef.current;
    const qs = new URLSearchParams();
    if (f.plugin) qs.set('plugin', f.plugin);
    if (f.status) qs.set('status', f.status);
    if (f.symbol) qs.set('symbol', f.symbol);
    qs.set('limit', '100');
    const r = await fetch('/api/execution/feedback?' + qs.toString(), { credentials: 'include' });
    if (r.ok) { const b = await r.json(); setFeedback(b.items ?? []); }
  }, []);

  useEffect(() => {
    void Promise.allSettled([loadConfig(), loadStats(), loadFeedback()]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const timerRef = useRef<number | null>(null);
  useEffect(() => {
    const tick = () => {
      if (document.visibilityState !== 'visible') return;
      void Promise.allSettled([loadStats(), loadFeedback()]);
    };
    timerRef.current = window.setInterval(tick, 30_000);
    return () => { if (timerRef.current) window.clearInterval(timerRef.current); };
  }, [loadStats, loadFeedback]);

  const putConfig = useCallback(async (next: ExecutionConfig): Promise<Response> => {
    const resp = await fetch('/api/execution/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(next),
    });
    if (resp.ok) {
      await loadConfig();
      await loadStats();
    }
    return resp;
  }, [loadConfig, loadStats]);

  // putConfig exposed for child components in future tasks
  void putConfig;

  return (
    <div className="execution-tab">
      <div data-testid="kill-switch">
        <KillSwitch
          killed={!!config?.killed_at}
          killedAt={config?.killed_at || null}
          onToggle={async () => {
            const currentlyKilled = !!config?.killed_at;
            const on = !currentlyKilled;
            await fetch('/api/execution/kill', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              credentials: 'include',
              body: JSON.stringify({ on }),
            });
            await loadConfig();
          }}
        />
      </div>
      <div data-testid="plugins-area">
        {config?.plugins.map(p => {
          const s = stats.find(x => x.plugin_id === p.name);
          return (
            <PluginCard
              key={p.name}
              plugin={p}
              stats={s}
              onEdit={() => { /* Task 16 wires modal open */ }}
              onDelete={async () => {
                if (!config) return;
                const nextPlugins = config.plugins.filter(x => x.name !== p.name);
                await putConfig({ ...config, plugins: nextPlugins });
              }}
              onToggleEnabled={async next => {
                if (!config) return;
                const nextPlugins = config.plugins.map(x => x.name === p.name ? { ...x, enabled: next } : x);
                await putConfig({ ...config, plugins: nextPlugins });
              }}
            />
          );
        })}
        <button onClick={() => { /* Task 16 wires Add modal */ }}>{t('execution.add_plugin')}</button>
      </div>
      <div data-testid="global-config">{/* GlobalConfigForm — Task 17 */}</div>
      <div data-testid="feedback-table">
        <FeedbackTable
          feedback={feedback}
          filters={filters}
          onFiltersChange={f => {
            filtersRef.current = f;  // sync ref FIRST so loadFeedback reads new filters
            setFilters(f);
            setFeedback([]);         // v2.4.0.2 regression guard — synchronous clear
            void loadFeedback();
          }}
          onRefresh={() => loadFeedback()}
          pluginNames={(config?.plugins ?? []).map(p => p.name)}
        />
      </div>
    </div>
  );
}
