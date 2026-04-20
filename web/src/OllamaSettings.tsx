import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

export type OllamaStatus = {
  state: 'READY' | 'READY_NO_MODEL' | 'INSTALLED_NOT_RUNNING' | 'NOT_INSTALLED' | 'DOCKER_SIDECAR_AVAILABLE';
  host: string;
  model: string;
  models_available?: string[];
  deployment: 'docker' | 'native';
  version?: string;
  suggest: { action: string; command?: string; size_bytes?: number };
};

type FetchResult =
  | { kind: 'ok'; data: OllamaStatus }
  | { kind: 'not_configured' }
  | { kind: 'error' };

type PullProgress = {
  status: string;
  completed?: number;
  total?: number;
  errorMessage?: string;
};

function formatBytes(bytes: number): string {
  if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(1)} GB`;
  if (bytes >= 1e6) return `${(bytes / 1e6).toFixed(1)} MB`;
  return `${bytes} B`;
}

const pillBase: React.CSSProperties = {
  display: 'inline-block',
  padding: '2px 8px',
  borderRadius: '12px',
  fontSize: '0.75rem',
  textTransform: 'uppercase',
  fontWeight: 600,
  letterSpacing: '0.05em',
};

const pillStyles: Record<OllamaStatus['state'], React.CSSProperties> = {
  READY: { ...pillBase, background: 'rgba(91,200,91,0.18)', color: 'var(--safe, #5bc85b)' },
  READY_NO_MODEL: { ...pillBase, background: 'rgba(255,180,50,0.18)', color: 'var(--warning, #ffb432)' },
  INSTALLED_NOT_RUNNING: { ...pillBase, background: 'rgba(255,200,50,0.18)', color: 'var(--warning, #ffc832)' },
  NOT_INSTALLED: { ...pillBase, background: 'rgba(255,68,68,0.18)', color: 'var(--danger, #ff4444)' },
  DOCKER_SIDECAR_AVAILABLE: { ...pillBase, background: 'rgba(148,163,184,0.18)', color: 'var(--slate)' },
};

const pillLabels: Record<OllamaStatus['state'], string> = {
  READY: 'ollama.state_ready',
  READY_NO_MODEL: 'ollama.state_no_model',
  INSTALLED_NOT_RUNNING: 'ollama.state_not_running',
  NOT_INSTALLED: 'ollama.state_not_installed',
  DOCKER_SIDECAR_AVAILABLE: 'ollama.state_sidecar_available',
};

type StateCardProps = {
  status: OllamaStatus;
  t: (k: string, o?: Record<string, string>) => string;
  pulling: PullProgress | null;
  onPull: () => void;
  onCancelPull: () => void;
  onResetPullError: () => void;
  pullInFlight: boolean;
};

function StateCard({ status, t, pulling, onPull, onCancelPull, onResetPullError, pullInFlight }: StateCardProps) {
  const { state, suggest } = status;

  if (state === 'READY') {
    return (
      <div style={{ marginTop: '0.75rem', display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
        <span style={pillStyles.READY}>{t(pillLabels.READY)}</span>
        <button className="tab-btn" disabled style={{ opacity: 0.4 }}>
          {t('ollama.test_connection')}
        </button>
      </div>
    );
  }

  if (state === 'READY_NO_MODEL') {
    const sizeStr = suggest.size_bytes != null ? formatBytes(suggest.size_bytes) : '';
    return (
      <div style={{ marginTop: '0.75rem' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
          <span style={pillStyles.READY_NO_MODEL}>{t(pillLabels.READY_NO_MODEL)}</span>
          {pulling === null && (
            <button className="tab-btn" onClick={onPull}>
              {t('ollama.pull_model')}
            </button>
          )}
          {sizeStr && pulling === null && (
            <span style={{ fontSize: '0.8rem', color: 'var(--muted)' }}>
              {t('ollama.download_size', { size: sizeStr })}
            </span>
          )}
        </div>

        {pulling !== null && pulling.status !== 'error' && (
          <div style={{ marginTop: 12 }}>
            <div style={{ fontSize: '0.8rem', color: 'var(--muted)', marginBottom: 4 }}>
              {pulling.status}
              {pulling.completed != null && pulling.total != null && (
                <> — {Math.floor((pulling.completed / pulling.total) * 100)}%</>
              )}
            </div>
            <div style={{ width: '100%', height: 8, background: 'rgba(255,255,255,0.08)', borderRadius: 4, overflow: 'hidden' }}>
              <div
                style={{
                  width: pulling.completed != null && pulling.total != null
                    ? `${(pulling.completed / pulling.total) * 100}%`
                    : '20%',
                  height: '100%',
                  background: 'var(--accent)',
                  transition: 'width 0.2s ease',
                }}
                role="progressbar"
                aria-valuenow={pulling.completed != null && pulling.total != null ? Math.floor((pulling.completed / pulling.total) * 100) : undefined}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-label={t('ollama.pull_progress')}
              />
            </div>
            <button
              type="button"
              onClick={onCancelPull}
              style={{ marginTop: 8, padding: '4px 10px', fontSize: '0.78rem' }}
            >
              {t('ollama.cancel_pull')}
            </button>
          </div>
        )}

        {pulling?.status === 'error' && (
          <div style={{ marginTop: 12, color: 'var(--danger)', fontSize: '0.85rem' }}>
            {pulling.errorMessage || 'Pull failed'}
            <button
              type="button"
              onClick={onResetPullError}
              style={{ marginLeft: 8, padding: '4px 10px', fontSize: '0.78rem' }}
            >
              {t('ollama.pull_try_again')}
            </button>
          </div>
        )}
      </div>
    );
  }

  if (state === 'INSTALLED_NOT_RUNNING') {
    return (
      <div style={{ marginTop: '0.75rem', display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
        <span style={pillStyles.INSTALLED_NOT_RUNNING}>{t(pillLabels.INSTALLED_NOT_RUNNING)}</span>
        <button className="tab-btn" disabled={pullInFlight} style={{ opacity: pullInFlight ? 0.4 : 1 }}>
          {t('ollama.start_ollama')}
        </button>
      </div>
    );
  }

  if (state === 'DOCKER_SIDECAR_AVAILABLE') {
    return (
      <div style={{ marginTop: '0.75rem', display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
        <span style={pillStyles.DOCKER_SIDECAR_AVAILABLE}>{t(pillLabels.DOCKER_SIDECAR_AVAILABLE)}</span>
        <button className="tab-btn" disabled={pullInFlight} style={{ opacity: pullInFlight ? 0.4 : 1 }}>
          {t('ollama.enable_sidecar')}
        </button>
      </div>
    );
  }

  // NOT_INSTALLED
  return (
    <div style={{ marginTop: '0.75rem' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap', marginBottom: '0.5rem' }}>
        <span style={pillStyles.NOT_INSTALLED}>{t(pillLabels.NOT_INSTALLED)}</span>
        <a
          href="https://ollama.com/download"
          target="_blank"
          rel="noopener noreferrer"
          className="tab-btn"
        >
          {t('ollama.install_ollama')}
        </a>
      </div>
      {suggest.command && (
        <div style={{ fontSize: '0.82rem', color: 'var(--muted)', marginTop: '0.4rem' }}>
          <code style={{
            background: 'rgba(255,255,255,0.06)',
            padding: '2px 6px',
            borderRadius: '4px',
            fontFamily: 'monospace',
          }}>
            {suggest.command}
          </code>
        </div>
      )}
    </div>
  );
}

export default function OllamaSettings() {
  const { t } = useTranslation();
  const [result, setResult] = useState<FetchResult | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const [pulling, setPulling] = useState<PullProgress | null>(null);
  const pullAbortRef = useRef<AbortController | null>(null);

  const fetchStatus = useCallback(async () => {
    try {
      const res = await fetch('/api/ai/ollama/status', { credentials: 'include' });
      if (res.status === 503) {
        setResult({ kind: 'not_configured' });
        return;
      }
      if (!res.ok) {
        setResult({ kind: 'error' });
        return;
      }
      const data: OllamaStatus = await res.json();
      setResult({ kind: 'ok', data });
    } catch {
      setResult({ kind: 'error' });
    }
  }, []);

  useEffect(() => {
    fetchStatus();

    const startPolling = () => {
      if (intervalRef.current) return;
      intervalRef.current = setInterval(() => {
        if (document.visibilityState === 'visible') {
          fetchStatus();
        }
      }, 5000);
    };

    const stopPolling = () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        startPolling();
      } else {
        stopPolling();
      }
    };

    if (document.visibilityState === 'visible') {
      startPolling();
    }

    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => {
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      stopPolling();
    };
  }, [fetchStatus]);

  const handlePull = useCallback(async () => {
    if (!result || result.kind !== 'ok') return;
    const status = result.data;
    const sizeFormatted = formatBytes(status.suggest.size_bytes ?? 0) || 'unknown size';
    const confirmMsg = t('ollama.pull_confirm', { model: status.model, size: sizeFormatted });
    if (!window.confirm(confirmMsg)) return;

    const ctrl = new AbortController();
    pullAbortRef.current = ctrl;
    setPulling({ status: 'starting' });
    let succeeded = false;

    try {
      const res = await fetch('/api/ai/ollama/pull', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ model: status.model }),
        signal: ctrl.signal,
      });
      if (!res.ok || !res.body) {
        setPulling({ status: 'error', errorMessage: `HTTP ${res.status}` });
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buf = '';
      let done = false;

      while (!done) {
        const { value, done: streamDone } = await reader.read();
        if (streamDone) break;
        buf += decoder.decode(value, { stream: true });

        // Split into SSE frames (delimiter: blank line, i.e., \n\n)
        let frameEnd: number;
        while ((frameEnd = buf.indexOf('\n\n')) !== -1) {
          const frame = buf.slice(0, frameEnd);
          buf = buf.slice(frameEnd + 2);

          // Detect 'event: done' frames
          if (frame.includes('event: done')) { done = true; succeeded = true; break; }

          // Extract data: <payload>
          const dataLine = frame.split('\n').find(l => l.startsWith('data: '));
          if (!dataLine) continue;
          const payload = dataLine.slice('data: '.length);
          let obj: Record<string, unknown>;
          try { obj = JSON.parse(payload); } catch { continue; }

          if (obj.error) {
            setPulling({ status: 'error', errorMessage: String(obj.error) });
            done = true;
            break;
          }

          const statusStr = typeof obj.status === 'string' ? obj.status : 'downloading';
          if (statusStr === 'success') {
            succeeded = true;
          }
          setPulling({
            status: statusStr,
            completed: typeof obj.completed === 'number' ? obj.completed : undefined,
            total: typeof obj.total === 'number' ? obj.total : undefined,
          });
        }
      }
    } catch (e) {
      // Abort or network error
      if ((e as Error).name === 'AbortError') {
        setPulling(null);
        return;
      }
      setPulling({ status: 'error', errorMessage: (e as Error).message });
      return;
    } finally {
      pullAbortRef.current = null;
    }

    // Only refresh status on genuine success (not on error frames).
    if (!succeeded) return;
    setPulling(null);
    await fetchStatus();
  }, [result, t, fetchStatus]);

  const handleCancelPull = useCallback(() => {
    pullAbortRef.current?.abort();
  }, []);

  const handleResetPullError = useCallback(() => {
    setPulling(null);
  }, []);

  const cardStyle: React.CSSProperties = {
    background: 'rgba(255,255,255,0.03)',
    border: '1px solid rgba(91,146,121,0.2)',
    borderRadius: '8px',
    padding: '1rem 1.25rem',
  };

  const labelStyle: React.CSSProperties = {
    fontSize: '0.78rem',
    color: 'var(--muted)',
    marginRight: '0.4rem',
  };

  const valueStyle: React.CSSProperties = {
    fontSize: '0.82rem',
    color: 'var(--text)',
  };

  const renderContent = () => {
    if (result === null) {
      return <p style={{ color: 'var(--muted)', fontSize: '0.85rem' }}>{t('ollama.loading')}</p>;
    }

    if (result.kind === 'error') {
      return (
        <div style={{ ...cardStyle, borderColor: 'rgba(255,68,68,0.35)' }}>
          <p style={{ color: 'var(--danger, #ff4444)', margin: 0 }}>{t('ollama.fetch_failed')}</p>
          <button
            className="tab-btn"
            onClick={fetchStatus}
            style={{ marginTop: '0.75rem' }}
          >
            {t('ollama.retry_status')}
          </button>
        </div>
      );
    }

    if (result.kind === 'not_configured') {
      return (
        <div style={cardStyle}>
          <p style={{ color: 'var(--muted)', margin: 0 }}>{t('ollama.detector_not_configured')}</p>
        </div>
      );
    }

    const { data } = result;
    const deploymentLabel = data.deployment === 'docker'
      ? t('ollama.deployment_docker')
      : t('ollama.deployment_native');

    return (
      <div style={cardStyle}>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '1rem', marginBottom: '0.25rem' }}>
          <span>
            <span style={labelStyle}>{t('ollama.host_label')}:</span>
            <span style={valueStyle}>{data.host}</span>
          </span>
          <span>
            <span style={labelStyle}>{t('ollama.model_label')}:</span>
            <span style={valueStyle}>{data.model}</span>
          </span>
          {data.version && (
            <span>
              <span style={labelStyle}>{t('ollama.version_label')}:</span>
              <span style={valueStyle}>{data.version}</span>
            </span>
          )}
          <span style={{
            ...pillBase,
            background: 'rgba(255,255,255,0.07)',
            color: 'var(--muted)',
          }}>
            {deploymentLabel}
          </span>
        </div>

        <StateCard
          status={data}
          t={t}
          pulling={pulling}
          onPull={handlePull}
          onCancelPull={handleCancelPull}
          onResetPullError={handleResetPullError}
          pullInFlight={pulling !== null}
        />

        <div style={{ marginTop: '1rem', borderTop: '1px solid rgba(255,255,255,0.06)', paddingTop: '0.75rem' }}>
          <button
            className="tab-btn"
            onClick={fetchStatus}
          >
            {t('ollama.retry_status')}
          </button>
        </div>
      </div>
    );
  };

  return (
    <div>
      <h3 style={{
        fontSize: '0.78rem',
        textTransform: 'uppercase',
        letterSpacing: '0.08em',
        color: 'var(--accent)',
        marginBottom: '0.75rem',
        borderBottom: '1px solid rgba(91,146,121,0.2)',
        paddingBottom: '0.4rem',
      }}>
        {t('settings.ai_provider_ollama')}
      </h3>
      {renderContent()}
    </div>
  );
}
