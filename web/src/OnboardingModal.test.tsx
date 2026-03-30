import { render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { OnboardingModal, ONBOARDING_DONE_KEY } from './OnboardingModal'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = globalThis as any

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) =>
      opts && typeof opts === 'object' ? `${key}:${JSON.stringify(opts)}` : key,
  }),
}))

vi.mock('./i18n', () => ({ default: { language: 'en' } }))

const mockClose = vi.fn()
const mockGoToSettings = vi.fn()

function renderModal() {
  return render(
    <OnboardingModal onClose={mockClose} onGoToSettings={mockGoToSettings} />
  )
}

// Fetch call order when modal mounts:
//   1. GET /api/settings/config  (checkAlerts on mount)
//   2+ = user-triggered calls

const noAlertConfig = { ok: true, json: async () => ({}) } as Response
const okAlertConfig = {
  ok: true,
  json: async () => ({ TELEGRAM_BOT_TOKEN: '123:abc' }),
} as Response

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.removeItem(ONBOARDING_DONE_KEY)
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

// ── 1. 모달 렌더링 ────────────────────────────────────────────────────────────
it('renders modal when no localStorage key is set', () => {
  g.fetch = vi.fn().mockResolvedValue(noAlertConfig)
  renderModal()
  expect(screen.getByRole('dialog')).toBeInTheDocument()
})

// ── 14. 빈 심볼 → Add 버튼 비활성화 ─────────────────────────────────────────
it('disables the Add button when symbol input is empty', () => {
  g.fetch = vi.fn().mockResolvedValue(noAlertConfig)
  renderModal()
  expect(screen.getByRole('button', { name: /onboarding\.step1_btn/i })).toBeDisabled()
})

// ── 2. Step 1 성공 (201) → step1 완료 상태 ──────────────────────────────────
it('marks step1 done and advances to step2 on 201 response', async () => {
  g.fetch = vi.fn()
    .mockResolvedValueOnce(noAlertConfig)                           // 1: GET /api/settings/config
    .mockResolvedValueOnce({ status: 201, ok: true } as Response)  // 2: POST /api/symbols

  const user = userEvent.setup()
  renderModal()

  await user.type(screen.getByRole('textbox'), 'AAPL')
  await user.click(screen.getByRole('button', { name: /onboarding\.step1_btn/i }))

  await waitFor(() => {
    // step1Done nav indicator shows ✓
    expect(screen.getAllByText('✓').length).toBeGreaterThan(0)
  })
})

// ── 3. Step 1 실패 (4xx) → 인라인 에러 표시 ─────────────────────────────────
it('shows inline error on non-201 response', async () => {
  g.fetch = vi.fn()
    .mockResolvedValueOnce(noAlertConfig)                         // 1: GET /api/settings/config
    .mockResolvedValueOnce({                                      // 2: POST /api/symbols (fail)
      status: 422, ok: false, text: async () => 'bad symbol',
    } as Response)

  const user = userEvent.setup()
  renderModal()

  await user.type(screen.getByRole('textbox'), 'BAD!')
  await user.click(screen.getByRole('button', { name: /onboarding\.step1_btn/i }))

  await waitFor(() => {
    expect(screen.getByRole('alert')).toBeInTheDocument()
  })
})

// ── 4. TELEGRAM 설정 → ok 배너 (step2에서 표시) ─────────────────────────────
it('shows ok banner when TELEGRAM_BOT_TOKEN is set', async () => {
  g.fetch = vi.fn()
    .mockResolvedValueOnce(okAlertConfig)                           // 1: GET /api/settings/config
    .mockResolvedValueOnce({ status: 201, ok: true } as Response)  // 2: POST /api/symbols

  const user = userEvent.setup()
  renderModal()

  await user.type(screen.getByRole('textbox'), 'AAPL')
  await user.click(screen.getByRole('button', { name: /onboarding\.step1_btn/i }))

  await waitFor(() => {
    expect(document.querySelector('.ob-alert-ok')).toBeInTheDocument()
  })
})

// ── 5. 미설정 → 경고 배너 (step2에서 표시) ──────────────────────────────────
it('shows warning banner when no alert channel is configured', async () => {
  // noAlertConfig is also what setupStep2 uses — advance to step2
  const user = await setupStep2()
  // step2 should now render the warning banner
  await waitFor(() => {
    expect(document.querySelector('.ob-alert-warning')).toBeInTheDocument()
  })
  // suppress unused warning
  void user
})

// ── 6. API 404 → unknown graceful (step2에서 표시) ───────────────────────────
it('shows unknown state when config API returns 404', async () => {
  g.fetch = vi.fn()
    .mockResolvedValueOnce({ status: 404, ok: false } as Response) // 1: GET /api/settings/config (404)
    .mockResolvedValueOnce({ status: 201, ok: true } as Response)  // 2: POST /api/symbols

  const user = userEvent.setup()
  renderModal()

  await user.type(screen.getByRole('textbox'), 'AAPL')
  await user.click(screen.getByRole('button', { name: /onboarding\.step1_btn/i }))

  await waitFor(() => {
    expect(document.querySelector('.ob-alert-unknown')).toBeInTheDocument()
  })
})

// Helper: advance to step2 (step1 done, scan panel visible)
async function setupStep2() {
  g.fetch = vi.fn()
    .mockResolvedValueOnce(noAlertConfig)                           // 1: GET /api/settings/config
    .mockResolvedValueOnce({ status: 201, ok: true } as Response)  // 2: POST /api/symbols

  const user = userEvent.setup()
  renderModal()

  await user.type(screen.getByRole('textbox'), 'AAPL')
  await user.click(screen.getByRole('button', { name: /onboarding\.step1_btn/i }))
  await waitFor(() => screen.getByRole('button', { name: /onboarding\.step2_btn/i }))
  return user
}

// ── 7. 200 + AI 데이터 → AI 카드 표시 ────────────────────────────────────────
it('shows AI result card on 200 with scenario data', async () => {
  const user = await setupStep2()

  vi.mocked(g.fetch as typeof fetch).mockResolvedValueOnce({
    ok: true,
    json: async () => ({ bull_pct: 60, bear_pct: 25, sideways_pct: 15 }),
  } as Response)

  await user.click(screen.getByRole('button', { name: /onboarding\.step2_btn/i }))

  await waitFor(() => {
    expect(document.querySelector('.ob-ai-card')).toBeInTheDocument()
  })
})

// ── 8. 503 → 기술적 시그널만 레이블 ─────────────────────────────────────────
it('shows technical-only label when scan returns 503', async () => {
  const user = await setupStep2()

  vi.mocked(g.fetch as typeof fetch).mockResolvedValueOnce({ status: 503, ok: false } as Response)

  await user.click(screen.getByRole('button', { name: /onboarding\.step2_btn/i }))

  await waitFor(() => {
    expect(document.querySelector('.ob-llm-note')).toBeInTheDocument()
  })
})

// ── 9. 타임아웃 60s → 에러 메시지 ───────────────────────────────────────────
it('shows timeout error message after 60 seconds', async () => {
  // Intercept the 60s abort timeout — avoids fake timer issues with React 18 scheduler
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const savedSetTimeout = (globalThis as any).setTimeout
  let triggerAbortTimeout!: () => void
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  ;(globalThis as any).setTimeout = (cb: () => void, ms?: number) => {
    if (ms === 60000) { triggerAbortTimeout = cb; return 0 }
    return savedSetTimeout(cb, ms)
  }

  g.fetch = vi.fn()
    .mockResolvedValueOnce(noAlertConfig)                           // 1: GET /api/settings/config
    .mockResolvedValueOnce({ status: 201, ok: true } as Response)  // 2: POST /api/symbols
    // 3: POST /api/analysis/full — rejects with AbortError when signal fires
    .mockImplementationOnce((_url: unknown, init?: RequestInit) =>
      new Promise<Response>((_, reject) => {
        init?.signal?.addEventListener('abort', () =>
          reject(new DOMException('The operation was aborted.', 'AbortError'))
        )
      })
    )

  try {
    const user = userEvent.setup()
    renderModal()

    await user.type(screen.getByRole('textbox'), 'AAPL')
    await user.click(screen.getByRole('button', { name: /onboarding\.step1_btn/i }))
    await waitFor(() => screen.getByRole('button', { name: /onboarding\.step2_btn/i }))
    await user.click(screen.getByRole('button', { name: /onboarding\.step2_btn/i }))

    // Restore real setTimeout, then fire the captured abort callback (≡ 60s elapsed)
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(globalThis as any).setTimeout = savedSetTimeout
    await act(async () => { triggerAbortTimeout() })

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
  } finally {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ;(globalThis as any).setTimeout = savedSetTimeout
  }
})

// ── 10. 완료 → localStorage 설정됨 ───────────────────────────────────────────
it('sets localStorage key to "1" after completing both steps', async () => {
  const user = await setupStep2()

  vi.mocked(g.fetch as typeof fetch).mockResolvedValueOnce({
    ok: true,
    json: async () => ({ bull_pct: 60, bear_pct: 25, sideways_pct: 15 }),
  } as Response)

  await user.click(screen.getByRole('button', { name: /onboarding\.step2_btn/i }))

  await waitFor(() => {
    expect(document.querySelector('.ob-completion')).toBeInTheDocument()
  })
  expect(localStorage.getItem(ONBOARDING_DONE_KEY)).toBe('1')
})

// ── 11. 스킵 → localStorage 미설정 ──────────────────────────────────────────
it('calls onClose without setting localStorage when skip is clicked', async () => {
  g.fetch = vi.fn().mockResolvedValue(noAlertConfig)

  const user = userEvent.setup()
  renderModal()

  await user.click(screen.getByRole('button', { name: /onboarding\.skip/i }))

  expect(mockClose).toHaveBeenCalledOnce()
  expect(localStorage.getItem(ONBOARDING_DONE_KEY)).toBeNull()
})

// ── 12. ESC → onClose ────────────────────────────────────────────────────────
it('calls onClose when Escape key is pressed', async () => {
  g.fetch = vi.fn().mockResolvedValue(noAlertConfig)

  const user = userEvent.setup()
  renderModal()

  await user.keyboard('{Escape}')

  expect(mockClose).toHaveBeenCalledOnce()
})

// ── 13. Twitter 공유 버튼 → confirm 다이얼로그 ───────────────────────────────
it('shows confirm dialog when share button is clicked', async () => {
  const user = await setupStep2()

  vi.mocked(g.fetch as typeof fetch).mockResolvedValueOnce({
    ok: true,
    json: async () => ({ bull_pct: 55, bear_pct: 30, sideways_pct: 15 }),
  } as Response)

  await user.click(screen.getByRole('button', { name: /onboarding\.step2_btn/i }))
  await waitFor(() => document.querySelector('.ob-completion'))

  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
  await user.click(screen.getByRole('button', { name: /onboarding\.share_twitter/i }))

  expect(confirmSpy).toHaveBeenCalledOnce()
})

// ── bonus: scan button disabled while loading ─────────────────────────────────
describe('scan loading state', () => {
  it('disables scan button while scan is in progress', async () => {
    const user = await setupStep2()

    let resolveFetch!: (v: Response) => void
    vi.mocked(g.fetch as typeof fetch).mockImplementationOnce(
      () => new Promise<Response>((resolve) => { resolveFetch = resolve })
    )

    await user.click(screen.getByRole('button', { name: /onboarding\.step2_btn/i }))

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /onboarding\.scanning/i })).toBeDisabled()
    })

    await act(async () => {
      resolveFetch({ ok: false, status: 500, text: async () => '' } as Response)
    })
  })
})
