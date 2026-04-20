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

function StateCard({ status, t }: { status: OllamaStatus; t: (k: string, o?: Record<string, string>) => string }) {
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
      <div style={{ marginTop: '0.75rem', display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
        <span style={pillStyles.READY_NO_MODEL}>{t(pillLabels.READY_NO_MODEL)}</span>
        <button className="tab-btn" disabled style={{ opacity: 0.4 }}>
          {t('ollama.pull_model')}
        </button>
        {sizeStr && (
          <span style={{ fontSize: '0.8rem', color: 'var(--muted)' }}>
            {t('ollama.download_size', { size: sizeStr })}
          </span>
        )}
      </div>
    );
  }

  if (state === 'INSTALLED_NOT_RUNNING') {
    return (
      <div style={{ marginTop: '0.75rem', display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
        <span style={pillStyles.INSTALLED_NOT_RUNNING}>{t(pillLabels.INSTALLED_NOT_RUNNING)}</span>
        <button className="tab-btn" disabled style={{ opacity: 0.4 }}>
          {t('ollama.start_ollama')}
        </button>
      </div>
    );
  }

  if (state === 'DOCKER_SIDECAR_AVAILABLE') {
    return (
      <div style={{ marginTop: '0.75rem', display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
        <span style={pillStyles.DOCKER_SIDECAR_AVAILABLE}>{t(pillLabels.DOCKER_SIDECAR_AVAILABLE)}</span>
        <button className="tab-btn" disabled style={{ opacity: 0.4 }}>
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

        <StateCard status={data} t={t} />

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
