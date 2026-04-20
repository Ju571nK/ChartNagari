import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, act, fireEvent } from '@testing-library/react';
import OllamaSettings from './OllamaSettings';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, string | number>) => {
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
        'ollama.pull_confirm': 'This will download {{model}} (≈{{size}}). Continue?',
        'ollama.pull_progress': 'Pull progress',
        'ollama.cancel_pull': 'Cancel',
        'ollama.pull_try_again': 'Try again',
        'ollama.pull_in_progress': 'Pull in progress',
        'ollama.unknown_size': 'unknown size',
        'ollama.starting': 'Starting…',
        'ollama.start_success': 'Started (pid {{pid}})',
        'ollama.sidecar_success': 'Sidecar configured — run command copied to clipboard',
        'ollama.sidecar_already_configured': 'Already configured',
        'ollama.try_again': 'Try again',
      };
      let s = map[k] ?? k;
      if (o) for (const [key, val] of Object.entries(o)) s = s.replace(`{{${key}}}`, String(val));
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
  // jsdom's window.confirm throws "Not implemented" and returns undefined (falsy).
  // Override it directly so pull handlers proceed past the confirmation gate.
  window.confirm = vi.fn(() => true);

  // Provide a clipboard stub for sidecar tests.
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
    writable: true,
    configurable: true,
  });
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

  it('Pull streams SSE frames and updates progress', async () => {
    function sseResponse(chunks: string[]): Response {
      const stream = new ReadableStream({
        async start(controller) {
          for (const chunk of chunks) {
            controller.enqueue(new TextEncoder().encode(chunk));
          }
          controller.close();
        },
      });
      return new Response(stream, {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      });
    }

    const readyAfterPull = {
      state: 'READY',
      host: 'http://localhost:11434',
      model: 'gemma4:4b',
      deployment: 'native',
      version: '0.3.1',
      suggest: { action: 'none' },
    };

    const fetchMock = vi.fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify(noModelStatus), { status: 200 })
      )
      .mockResolvedValueOnce(
        sseResponse([
          'data: {"status":"pulling manifest"}\n\n',
          'data: {"status":"downloading","completed":25,"total":100}\n\n',
          'data: {"status":"downloading","completed":75,"total":100}\n\n',
          'data: {"status":"success"}\n\nevent: done\ndata: {}\n\n',
        ])
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(readyAfterPull), { status: 200 })
      );

    vi.stubGlobal('fetch', fetchMock);
    render(<OllamaSettings />);

    // Wait for initial status to render the Pull model button
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /pull model/i })).toBeInTheDocument()
    );

    fireEvent.click(screen.getByRole('button', { name: /pull model/i }));

    // Progress bar should appear
    await waitFor(() =>
      expect(screen.getByRole('progressbar')).toBeInTheDocument()
    );

    // Final state: READY card renders after status refetch
    await waitFor(() =>
      expect(screen.getByText('Ready — model loaded')).toBeInTheDocument(),
      { timeout: 3000 }
    );
  });

  it('Pull shows error on {"error":...} frame', async () => {
    function sseResponse(chunks: string[]): Response {
      const stream = new ReadableStream({
        async start(controller) {
          for (const chunk of chunks) {
            controller.enqueue(new TextEncoder().encode(chunk));
          }
          controller.close();
        },
      });
      return new Response(stream, {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      });
    }

    const fetchMock = vi.fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify(noModelStatus), { status: 200 })
      )
      .mockResolvedValueOnce(
        sseResponse([
          'data: {"status":"pulling manifest"}\n\n',
          'data: {"error":"disk quota exceeded"}\n\n',
        ])
      )
      // Persistent fallback for interval polls after the pull completes
      .mockResolvedValue(
        new Response(JSON.stringify(noModelStatus), { status: 200 })
      );

    vi.stubGlobal('fetch', fetchMock);
    render(<OllamaSettings />);

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /pull model/i })).toBeInTheDocument()
    );

    fireEvent.click(screen.getByRole('button', { name: /pull model/i }));

    // Error message and Try again button should appear
    await waitFor(() =>
      expect(screen.getByText('disk quota exceeded')).toBeInTheDocument(),
      { timeout: 3000 }
    );
    expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument();
  });

  it('Pull cancel aborts the request and clears progress UI', async () => {
    // In jsdom, aborting a fetch signal after reader.read() is pending does not
    // throw AbortError through the ReadableStream reader. Instead we simulate
    // abort by making the pull fetch return a promise that rejects with AbortError
    // when the signal fires — matching real browser behaviour.
    const fetchMock = vi.fn().mockImplementation((url: string, opts?: RequestInit) => {
      if (url === '/api/ai/ollama/status') {
        return Promise.resolve(new Response(JSON.stringify(noModelStatus), { status: 200 }));
      }
      // Pull endpoint: return a promise that rejects with AbortError on abort
      return new Promise<Response>((_resolve, reject) => {
        const signal = opts?.signal as AbortSignal | undefined;
        if (signal?.aborted) {
          const err = new DOMException('Aborted', 'AbortError');
          reject(err);
          return;
        }
        signal?.addEventListener('abort', () => {
          const err = new DOMException('Aborted', 'AbortError');
          reject(err);
        });
      });
    });

    vi.stubGlobal('fetch', fetchMock);
    render(<OllamaSettings />);

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /pull model/i })).toBeInTheDocument()
    );

    fireEvent.click(screen.getByRole('button', { name: /pull model/i }));

    // While fetch is pending, pulling state is 'starting' — progress bar and
    // Cancel button both appear immediately (indeterminate state, width: 20%)
    await waitFor(() =>
      expect(screen.getByRole('progressbar')).toBeInTheDocument()
    );
    expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument();

    // Click Cancel — triggers abort
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }));

    // Progress UI should disappear and Pull button should come back
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /pull model/i })).toBeInTheDocument(),
      { timeout: 3000 }
    );
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  });

  // ── Start button tests ───────────────────────────────────────────────────

  it('Start 200: shows success message then refetches status', async () => {
    const readyAfterStart = { ...notRunningStatus, state: 'READY', suggest: { action: 'none' } };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(notRunningStatus), { status: 200 }))
      .mockResolvedValueOnce(new Response('{}', { status: 200 })) // POST /start
      .mockResolvedValueOnce(new Response(JSON.stringify(readyAfterStart), { status: 200 })); // refetch

    vi.stubGlobal('fetch', fetchMock);
    render(<OllamaSettings />);

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /start ollama/i })).toBeInTheDocument()
    );

    fireEvent.click(screen.getByRole('button', { name: /start ollama/i }));

    await waitFor(() =>
      expect(screen.getByText(/started \(pid/i)).toBeInTheDocument()
    );

    expect(fetchMock).toHaveBeenCalledWith('/api/ai/ollama/start', expect.objectContaining({ method: 'POST' }));
  });

  it('Start 409: no error shown, silently refetches status', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(notRunningStatus), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'already running' }), { status: 409 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(notRunningStatus), { status: 200 }));

    vi.stubGlobal('fetch', fetchMock);
    render(<OllamaSettings />);

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /start ollama/i })).toBeInTheDocument()
    );

    fireEvent.click(screen.getByRole('button', { name: /start ollama/i }));

    // Wait for refetch to settle — button is back, no error text
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /start ollama/i })).toBeInTheDocument()
    );
    expect(screen.queryByText(/error/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/already running/i)).not.toBeInTheDocument();
  });

  it('Start 500: shows inline error with Try again button', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(notRunningStatus), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'did not become ready' }), { status: 500 }));

    vi.stubGlobal('fetch', fetchMock);
    render(<OllamaSettings />);

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /start ollama/i })).toBeInTheDocument()
    );

    fireEvent.click(screen.getByRole('button', { name: /start ollama/i }));

    await waitFor(() =>
      expect(screen.getByText('did not become ready')).toBeInTheDocument()
    );
    expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument();
  });

  it('Start Try again: resets error and shows Start button again', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(notRunningStatus), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'did not become ready' }), { status: 500 }));

    vi.stubGlobal('fetch', fetchMock);
    render(<OllamaSettings />);

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /start ollama/i })).toBeInTheDocument()
    );

    fireEvent.click(screen.getByRole('button', { name: /start ollama/i }));

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument()
    );

    fireEvent.click(screen.getByRole('button', { name: /try again/i }));

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /start ollama/i })).toBeInTheDocument()
    );
    expect(screen.queryByText('did not become ready')).not.toBeInTheDocument();
  });

  // ── Sidecar button tests ──────────────────────────────────────────────────

  it('Sidecar 200: copies run_command to clipboard and shows success with code block', async () => {
    const runCommand = 'docker compose up -d ollama';
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(sidecarStatus), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ override_path: '/docker-compose.yml', run_command: runCommand }), { status: 200 }));

    vi.stubGlobal('fetch', fetchMock);
    render(<OllamaSettings />);

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /enable docker sidecar/i })).toBeInTheDocument()
    );

    fireEvent.click(screen.getByRole('button', { name: /enable docker sidecar/i }));

    await waitFor(() =>
      expect(screen.getByText('Sidecar configured — run command copied to clipboard')).toBeInTheDocument()
    );

    expect(screen.getByText(runCommand)).toBeInTheDocument();
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(runCommand);
  });

  it('Sidecar 409: shows already-configured message with run command', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(sidecarStatus), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'override file already exists' }), { status: 409 }));

    vi.stubGlobal('fetch', fetchMock);
    render(<OllamaSettings />);

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /enable docker sidecar/i })).toBeInTheDocument()
    );

    fireEvent.click(screen.getByRole('button', { name: /enable docker sidecar/i }));

    await waitFor(() =>
      expect(screen.getByText('Already configured')).toBeInTheDocument()
    );

    expect(screen.getByText('docker compose up -d ollama')).toBeInTheDocument();
  });

  it('Sidecar 500: shows inline error with Try again button', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(sidecarStatus), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'internal sidecar error' }), { status: 500 }));

    vi.stubGlobal('fetch', fetchMock);
    render(<OllamaSettings />);

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /enable docker sidecar/i })).toBeInTheDocument()
    );

    fireEvent.click(screen.getByRole('button', { name: /enable docker sidecar/i }));

    await waitFor(() =>
      expect(screen.getByText('internal sidecar error')).toBeInTheDocument()
    );
    expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument();
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
