import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import KillSwitch from './KillSwitch';
import PluginCard from './PluginCard';
import PluginEditModal from './PluginEditModal';
import FeedbackTable from './FeedbackTable';
import GlobalConfigForm, { type GlobalConfig } from './GlobalConfigForm';

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
  const [editing, setEditing] = useState<Plugin | null>(null);
  const [editingOpen, setEditingOpen] = useState(false);
  const [versionConflict, setVersionConflict] = useState(false);
  const [serverFieldErrors, setServerFieldErrors] = useState<Record<string, string> | null>(null);
  const [configError, setConfigError] = useState(false);
  const [actionError, setActionError] = useState(false);
  const [feedError, setFeedError] = useState(false);

  const loadConfig = useCallback(async () => {
    try {
      const r = await fetch('/api/execution/config', { credentials: 'include' });
      if (!r.ok) throw new Error('Configuration unavailable');
      setConfig(await r.json());
      setConfigError(false);
    } catch (error) {
      setConfigError(true);
      setConfig(null);
      throw error;
    }
  }, []);

  const loadStats = useCallback(async () => {
    const r = await fetch('/api/execution/plugins/stats?window=24h', { credentials: 'include' });
    if (r.ok) { const b = await r.json(); setStats(b.plugins ?? []); }
  }, []);

  const loadFeedback = useCallback(async () => {
    try {
    const f = filtersRef.current;
    const qs = new URLSearchParams();
    if (f.plugin) qs.set('plugin', f.plugin);
    if (f.status) qs.set('status', f.status);
    if (f.symbol) qs.set('symbol', f.symbol);
    qs.set('limit', '100');
    const r = await fetch('/api/execution/feedback?' + qs.toString(), { credentials: 'include' });
    if (!r.ok) throw new Error('Feedback unavailable');
    const b = await r.json(); setFeedback(b.items ?? []); setFeedError(false);
    } catch { setFeedError(true); }
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
    setActionError(false);
    try {
    const resp = await fetch('/api/execution/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(next),
    });
    if (resp.status === 409) {
      setVersionConflict(true);
      return resp;
    }
    if (resp.ok) {
      await loadConfig();
      await loadStats();
    }
    if (!resp.ok && resp.status !== 422) setActionError(true);
    return resp;
    } catch {
      setActionError(true);
      return new Response(null, { status: 503 });
    }
  }, [loadConfig, loadStats]);

  return (
    <div className="execution-tab">
      {actionError && <div className="state-message" role="alert">{t('workspace.actionFailed')}</div>}
      {configError && <div className="state-message" role="alert"><p>{t('workspace.executionUnavailable')}</p><button onClick={() => { void loadConfig().catch(() => {}); }}>{t('workspace.retry')}</button></div>}
      {!config && !configError && <p className="state-message" role="status">{t('loading')}</p>}
      {config && <p role="status">{t(config.killed_at ? 'workspace.executionKilled' : config.enabled ? 'workspace.executionOn' : 'workspace.executionOff')}</p>}
      {versionConflict && (
        <div role="alert" style={{ background: 'var(--danger)', color: '#fff', padding: 12, marginBottom: 12, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{t('execution.config_conflict')}</span>
          <button onClick={async () => { try { await loadConfig(); setVersionConflict(false); } catch { /* config error is visible */ } }}>{t('common.refresh')}</button>
        </div>
      )}
      <div data-testid="kill-switch">
        {config && <KillSwitch
          killed={!!config?.killed_at}
          killedAt={config?.killed_at || null}
          onToggle={async () => {
            const currentlyKilled = !!config?.killed_at;
            const on = !currentlyKilled;
            const response = await fetch('/api/execution/kill', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              credentials: 'include',
              body: JSON.stringify({ on }),
            });
            if (!response.ok) throw new Error(t('workspace.actionFailed'));
            await loadConfig();
          }}
        />}
      </div>
      <div data-testid="plugins-area">
        {config?.plugins.map(p => {
          const s = stats.find(x => x.plugin_id === p.name);
          return (
            <PluginCard
              key={p.name}
              plugin={p}
              stats={s}
              onEdit={() => { setEditing(p); setEditingOpen(true); }}
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
        <button disabled={!config} onClick={() => { setEditing(null); setEditingOpen(true); }}>{t('execution.add_plugin')}</button>
      </div>
      <div data-testid="global-config">
        {config && (
          <GlobalConfigForm
            config={{
              max_dispatched: config.max_dispatched,
              dedup_window: config.dedup_window,
              symbol_map: config.symbol_map,
            }}
            onServerError={serverFieldErrors}
            onSave={async (partial: GlobalConfig) => {
              if (!config) return;
              const next = { ...config, ...partial };
              const resp = await putConfig(next);
              if (resp.status === 422) {
                try {
                  const body = await resp.json();
                  setServerFieldErrors(body.fields ?? null);
                } catch { setServerFieldErrors(null); }
                return;
              }
              if (resp.status === 409) {
                setVersionConflict(true);
                return;
              }
              setServerFieldErrors(null);
            }}
          />
        )}
      </div>
      {editingOpen && (
        <PluginEditModal
          plugin={editing}
          existingNames={(config?.plugins ?? []).map(p => p.name).filter(n => n !== editing?.name)}
          onCancel={() => setEditingOpen(false)}
          onSave={async next => {
            if (!config) return;
            const plugins = editing && editing.name
              ? config.plugins.map(p => p.name === editing.name ? next : p)
              : [...config.plugins, next];
            const resp = await putConfig({ ...config, plugins });
            if (resp.ok) setEditingOpen(false);
            else if (resp.status === 409) setEditingOpen(false); // close so banner is visible
          }}
        />
      )}
      <div data-testid="feedback-table">
        {feedError && <p className="state-message" role="alert">{t('workspace.actionFailed')}</p>}
        <FeedbackTable
          unavailable={feedError}
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
