import { useCallback, useEffect, useRef, useState } from 'react';

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
  const [config, setConfig] = useState<ExecutionConfig | null>(null);
  const [stats, setStats] = useState<PluginStat[]>([]);
  const [feedback, setFeedback] = useState<FeedbackRow[]>([]);
  const [filters, setFilters] = useState<FeedbackFilters>({ plugin: '', status: '', symbol: '' });

  const loadConfig = useCallback(async () => {
    const r = await fetch('/api/execution/config', { credentials: 'include' });
    if (r.ok) setConfig(await r.json());
  }, []);

  const loadStats = useCallback(async () => {
    const r = await fetch('/api/execution/plugins/stats?window=24h', { credentials: 'include' });
    if (r.ok) { const b = await r.json(); setStats(b.plugins ?? []); }
  }, []);

  const loadFeedback = useCallback(async (f: FeedbackFilters = filters) => {
    const qs = new URLSearchParams();
    if (f.plugin) qs.set('plugin', f.plugin);
    if (f.status) qs.set('status', f.status);
    if (f.symbol) qs.set('symbol', f.symbol);
    qs.set('limit', '100');
    const r = await fetch('/api/execution/feedback?' + qs.toString(), { credentials: 'include' });
    if (r.ok) { const b = await r.json(); setFeedback(b.items ?? []); }
  }, [filters]);

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

  // Expose setFilters for child components in future tasks
  void setFilters;
  void config;
  void stats;
  void feedback;

  return (
    <div className="execution-tab">
      <div data-testid="kill-switch">{/* KillSwitch — Task 13 */}</div>
      <div data-testid="plugins-area">{/* PluginCard list + Add — Tasks 14/16 */}</div>
      <div data-testid="global-config">{/* GlobalConfigForm — Task 17 */}</div>
      <div data-testid="feedback-table">{/* FeedbackTable — Task 15 */}</div>
    </div>
  );
}
