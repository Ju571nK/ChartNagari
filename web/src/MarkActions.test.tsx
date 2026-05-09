import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import i18n from './i18n'
import { MarkActions } from './MarkActions'

const wrap = (ui: React.ReactElement) => (
  <I18nextProvider i18n={i18n}>{ui}</I18nextProvider>
)

beforeEach(() => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({ signal_id: 1, status: 'TOOK', updated_at: 0 }),
  } as Response)
})
afterEach(() => { vi.restoreAllMocks() })

describe('MarkActions', () => {
  it('PENDING shows Took + Skipped', () => {
    render(wrap(<MarkActions signalId={1} status="PENDING" />))
    expect(screen.getByRole('button', { name: /Took|들어감|エントリー/ })).toBeDefined()
    expect(screen.getByRole('button', { name: /Skipped|스킵|スキップ/ })).toBeDefined()
  })

  it('TOOK shows Win/Loss/BE/Undo', () => {
    render(wrap(<MarkActions signalId={1} status="TOOK" />))
    expect(screen.getByRole('button', { name: /Win|익절|勝ち/ })).toBeDefined()
    expect(screen.getByRole('button', { name: /Loss|손절|負け/ })).toBeDefined()
    expect(screen.getByRole('button', { name: /BE/ })).toBeDefined()
    expect(screen.getByRole('button', { name: /Undo|되돌리기|戻す/ })).toBeDefined()
  })

  it('clicking Took fires POST and calls onMarked', async () => {
    const onMarked = vi.fn()
    render(wrap(<MarkActions signalId={42} status="PENDING" onMarked={onMarked} />))
    const btn = screen.getByRole('button', { name: /Took|들어감|エントリー/ })
    fireEvent.click(btn)
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled())
    const lastCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(lastCall[0]).toBe('/api/marks/42')
    expect((lastCall[1] as RequestInit).method).toBe('POST')
    const body = JSON.parse((lastCall[1] as RequestInit).body as string)
    expect(body.action).toBe('took')
    await waitFor(() => expect(onMarked).toHaveBeenCalledWith('TOOK'))
  })

  it('rolls back on POST failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false, status: 500, json: async () => ({ error: 'boom' }),
    } as Response)
    const onMarked = vi.fn()
    render(wrap(<MarkActions signalId={1} status="PENDING" onMarked={onMarked} />))
    fireEvent.click(screen.getByRole('button', { name: /Took|들어감|エントリー/ }))
    await waitFor(() => expect(screen.queryByText(/Save failed|저장 실패|保存失敗/)).toBeDefined())
    expect(onMarked).not.toHaveBeenCalled()
  })
})
