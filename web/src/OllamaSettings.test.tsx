import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, act } from '@testing-library/react';
import OllamaSettings from './OllamaSettings';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, string>) => {
      const map: Record<string, string> = {
        'settings.ai_provider_ollama': 'AI Provider (Ollama)',
        'ollama.state_ready': 'Ready — model loaded',
        'ollama.state_no_model': 'Ready, model not pulled',
        'ollama.state_not_running': 'Installed, not running',
        'ollama.state_not_installed': 'Not installed',
        'ollama.state_sidecar_available': 'Docker sidecar available',
        'ollama.pull_model': 'Pull model',
        'ollama.start_ollama': 'Start Ollama',
        'ollama.install_ollama': 'Install Ollama',
        'ollama.enable_sidecar': 'Enable Docker sidecar',
        'ollama.loading': 'Loading\u2026',
        'ollama.test_connection': 'Test connection',
        'ollama.retry_status': 'Refresh status',
        'ollama.detector_not_configured': 'Ollama detector not configured on the server',
        'ollama.fetch_failed': 'Failed to reach server',
        'ollama.host_label': 'Host',
        'ollama.model_label': 'Model',
        'ollama.version_label': 'Version',
        'ollama.deployment_native': 'Native',
        'ollama.deployment_docker': 'Docker',
        'ollama.download_size': 'Download size: {{size}}',
      };
      let s = map[k] ?? k;
      if (o) for (const [key, val] of Object.entries(o)) s = s.replace(`{{${key}}}`, val);
      return s;
    },
  }),
}));

vi.mock('./i18n/index', () => ({ default: { language: 'en' } }));

const readyStatus = {
  state: 'READY',
  host: 'http://localhost:11434',
  model: 'gemma4:4b',
  deployment: 'native',
  version: '0.3.1',
  suggest: { action: 'none' },
};

const noModelStatus = {
  state: 'READY_NO_MODEL',
  host: 'http://localhost:11434',
  model: 'gemma4:4b',
  deployment: 'native',
  suggest: { action: 'pull', size_bytes: 2600000000 },
};

const notRunningStatus = {
  state: 'INSTALLED_NOT_RUNNING',
  host: 'http://localhost:11434',
  model: 'gemma4:4b',
  deployment: 'native',
  suggest: { action: 'start', command: 'ollama serve' },
};

const sidecarStatus = {
  state: 'DOCKER_SIDECAR_AVAILABLE',
  host: 'http://ollama:11434',
  model: 'gemma4:4b',
  deployment: 'docker',
  suggest: { action: 'enable_sidecar' },
};

const notInstalledStatus = {
  state: 'NOT_INSTALLED',
  host: 'http://localhost:11434',
  model: 'gemma4:4b',
  deployment: 'native',
  suggest: { action: 'install', command: 'curl -fsSL https://ollama.com/install.sh | sh' },
};

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('OllamaSettings', () => {
  it('renders READY state with green pill and Test connection button', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce(
      new Response(JSON.stringify(readyStatus), { status: 200 })
    );

    render(<OllamaSettings />);

    await waitFor(() => {
      expect(screen.getByText('Ready — model loaded')).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /test connection/i })).toBeInTheDocument();
  });

  it('renders READY_NO_MODEL with Pull model button and formatted size', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce(
      new Response(JSON.stringify(noModelStatus), { status: 200 })
    );

    render(<OllamaSettings />);

    await waitFor(() => {
      expect(screen.getByText('Ready, model not pulled')).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /pull model/i })).toBeInTheDocument();
    expect(screen.getByText(/2\.6 GB/)).toBeInTheDocument();
  });

  it('renders INSTALLED_NOT_RUNNING with Start Ollama button', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce(
      new Response(JSON.stringify(notRunningStatus), { status: 200 })
    );

    render(<OllamaSettings />);

    await waitFor(() => {
      expect(screen.getByText('Installed, not running')).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /start ollama/i })).toBeInTheDocument();
  });

  it('renders DOCKER_SIDECAR_AVAILABLE with Enable Docker sidecar button', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce(
      new Response(JSON.stringify(sidecarStatus), { status: 200 })
    );

    render(<OllamaSettings />);

    await waitFor(() => {
      expect(screen.getByText('Docker sidecar available')).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /enable docker sidecar/i })).toBeInTheDocument();
  });

  it('renders NOT_INSTALLED with suggest.command in code block and Install Ollama link', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce(
      new Response(JSON.stringify(notInstalledStatus), { status: 200 })
    );

    render(<OllamaSettings />);

    await waitFor(() => {
      expect(screen.getByText('Not installed')).toBeInTheDocument();
    });

    expect(screen.getByText('curl -fsSL https://ollama.com/install.sh | sh')).toBeInTheDocument();

    const installLink = screen.getByRole('link', { name: /install ollama/i });
    expect(installLink).toBeInTheDocument();
    expect(installLink).toHaveAttribute('href', 'https://ollama.com/download');
    expect(installLink).toHaveAttribute('target', '_blank');
  });

  it('renders detector-not-configured message on 503 without a state card', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce(
      new Response('', { status: 503 })
    );

    render(<OllamaSettings />);

    await waitFor(() => {
      expect(screen.getByText('Ollama detector not configured on the server')).toBeInTheDocument();
    });

    // No pill states should be shown
    expect(screen.queryByText('Ready — model loaded')).not.toBeInTheDocument();
    expect(screen.queryByText('Not installed')).not.toBeInTheDocument();
  });

  it('renders fetch-failed banner with retry button on network error', async () => {
    globalThis.fetch = vi.fn().mockRejectedValueOnce(new Error('Network error'));

    render(<OllamaSettings />);

    await waitFor(() => {
      expect(screen.getByText('Failed to reach server')).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /refresh status/i })).toBeInTheDocument();
  });

  it('polls every 5s only when document is visible', async () => {
    vi.useFakeTimers();

    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(readyStatus), { status: 200 })
    );
    globalThis.fetch = fetchMock;

    // Ensure document is visible
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => 'visible',
    });

    await act(async () => {
      render(<OllamaSettings />);
    });

    // Initial fetch on mount
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // Advance 5 seconds — should trigger one poll
    await act(async () => {
      vi.advanceTimersByTime(5000);
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);

    // Simulate hidden — next tick should NOT increment
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => 'hidden',
    });

    // Trigger the visibilitychange so the component stops the interval
    act(() => {
      document.dispatchEvent(new Event('visibilitychange'));
    });

    await act(async () => {
      vi.advanceTimersByTime(5000);
    });

    // Should still be 2 — poll was stopped
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
