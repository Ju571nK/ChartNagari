import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import MCPSettings from './MCPSettings'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, string>) => {
      const map: Record<string, string> = {
        'settings.mcp_server': 'MCP server',
        'mcp.status_active': 'Active',
        'mcp.tools_count': '{{n}} tools registered',
        'mcp.client_claude_code': 'Claude Code',
        'mcp.client_claude_desktop': 'Claude Desktop',
        'mcp.client_codex': 'Codex CLI',
        'mcp.copy': 'Copy',
        'mcp.copied': 'Copied!',
        'mcp.token_masked': '{{mask}}',
      }
      let s = map[k] ?? k
      if (o) for (const [key, val] of Object.entries(o)) s = s.replace(`{{${key}}}`, String(val))
      return s
    },
  }),
}))

beforeEach(() => {
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
    writable: true, configurable: true,
  })
})

describe('MCPSettings', () => {
  it('renders MCP endpoint + tool count + active status', () => {
    render(<MCPSettings apiToken="secret123" endpointURL="http://localhost:8080/api/mcp" toolNames={["get_analysis","list_watchlist"]} />)
    expect(screen.getByText(/MCP server/i)).toBeInTheDocument()
    // endpoint displayed in the <code> label (not just in the snippet <pre>)
    const endpointCode = screen.getByText('http://localhost:8080/api/mcp', { selector: 'code' })
    expect(endpointCode).toBeInTheDocument()
    expect(screen.getByText(/2 tools registered/i)).toBeInTheDocument()
  })

  it('masks token until revealed', () => {
    render(<MCPSettings apiToken="abcd1234efgh5678" endpointURL="..." toolNames={[]} />)
    // Full token must not appear as its own text node
    expect(screen.queryByText('abcd1234efgh5678')).toBeNull()
    // Masked form shown in the token <code> element
    const tokenCode = screen.getByText(/abcd.*5678/i, { selector: 'code' })
    expect(tokenCode).toBeInTheDocument()
  })

  it('renders three client snippet tabs', () => {
    render(<MCPSettings apiToken="x" endpointURL="y" toolNames={[]} />)
    expect(screen.getByRole('button', { name: /claude code/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /claude desktop/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /codex cli/i })).toBeInTheDocument()
  })

  it('copies the selected client snippet to clipboard', async () => {
    render(<MCPSettings apiToken="tok123" endpointURL="http://localhost:8080/api/mcp" toolNames={[]} />)
    const copyBtn = screen.getAllByRole('button', { name: /copy/i })[0]
    fireEvent.click(copyBtn)
    expect((navigator.clipboard.writeText as any).mock.calls.length).toBe(1)
    const call = (navigator.clipboard.writeText as any).mock.calls[0][0] as string
    expect(call).toContain('tok123')
  })

  it('shows Codex TOML when Codex tab selected', () => {
    render(<MCPSettings apiToken="tok" endpointURL="http://localhost:8080/api/mcp" toolNames={[]} />)
    fireEvent.click(screen.getByRole('button', { name: /codex cli/i }))
    expect(screen.getByText(/\[\[mcp_servers\]\]/i)).toBeInTheDocument()
    expect(screen.getByText(/CHARTNAGARI_URL/i)).toBeInTheDocument()
  })
})
