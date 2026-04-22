import { useState } from 'react'
import { useTranslation } from 'react-i18next'

type Props = {
  apiToken: string
  endpointURL: string
  toolNames: string[]
}

type Tab = 'claude-code' | 'claude-desktop' | 'codex'

function maskToken(t: string): string {
  if (t.length < 8) return '••••'
  return t.slice(0, 4) + '…' + t.slice(-4)
}

function claudeCodeSnippet(url: string, token: string): string {
  return `claude mcp add --transport http chartnagari ${url} \\
  --header "Authorization: Bearer ${token}"`
}

function claudeDesktopSnippet(url: string, token: string): string {
  return JSON.stringify(
    {
      mcpServers: {
        chartnagari: {
          type: 'http',
          url,
          headers: { Authorization: `Bearer ${token}` },
        },
      },
    },
    null,
    2
  )
}

function codexSnippet(baseURL: string, token: string): string {
  const base = baseURL.replace(/\/api\/mcp$/, '')
  return `[[mcp_servers]]
name = "chartnagari"
command = "chartnagari-mcp"

[mcp_servers.env]
CHARTNAGARI_URL = "${base}"
CHARTNAGARI_TOKEN = "${token}"`
}

export default function MCPSettings({ apiToken, endpointURL, toolNames }: Props) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<Tab>('claude-code')
  const [revealed, setRevealed] = useState(false)
  const [copied, setCopied] = useState(false)

  const snippet =
    tab === 'claude-code' ? claudeCodeSnippet(endpointURL, apiToken)
    : tab === 'claude-desktop' ? claudeDesktopSnippet(endpointURL, apiToken)
    : codexSnippet(endpointURL, apiToken)

  const handleCopy = async () => {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(snippet)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }

  const pillStyle = {
    padding: '2px 8px', borderRadius: 12, fontSize: '0.75rem',
    textTransform: 'uppercase' as const,
    background: 'rgba(91,200,91,0.18)', color: 'var(--safe)',
  }

  return (
    <div style={{ marginTop: '2rem', paddingTop: '1.5rem', borderTop: '1px solid rgba(91,146,121,0.2)' }}>
      <h3 style={{ fontSize: '0.78rem', textTransform: 'uppercase', letterSpacing: '0.08em',
                   color: 'var(--accent)', marginBottom: '0.75rem' }}>
        {t('settings.mcp_server')}
      </h3>

      <div style={{ fontSize: '0.85rem', color: 'var(--muted)', marginBottom: 8 }}>
        <strong>Endpoint:</strong> <code>{endpointURL}</code>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
        <span style={pillStyle}>● {t('mcp.status_active')}</span>
        <span style={{ fontSize: '0.78rem', color: 'var(--muted)' }}>
          {t('mcp.tools_count', { n: String(toolNames.length) })}
        </span>
      </div>
      <div style={{ fontSize: '0.78rem', color: 'var(--muted)', marginBottom: 12 }}>
        Token: <code onClick={() => setRevealed(v => !v)} style={{ cursor: 'pointer' }}>
          {revealed ? apiToken : maskToken(apiToken)}
        </code>
      </div>

      <div style={{ display: 'flex', gap: 4, marginBottom: 8 }}>
        <button type="button" className="tab-btn" onClick={() => setTab('claude-code')}
                style={tab === 'claude-code' ? { background: 'var(--accent)' } : {}}>
          {t('mcp.client_claude_code')}
        </button>
        <button type="button" className="tab-btn" onClick={() => setTab('claude-desktop')}
                style={tab === 'claude-desktop' ? { background: 'var(--accent)' } : {}}>
          {t('mcp.client_claude_desktop')}
        </button>
        <button type="button" className="tab-btn" onClick={() => setTab('codex')}
                style={tab === 'codex' ? { background: 'var(--accent)' } : {}}>
          {t('mcp.client_codex')}
        </button>
      </div>

      <pre style={{ background: 'rgba(255,255,255,0.06)', padding: 12, borderRadius: 4,
                    fontSize: '0.78rem', overflow: 'auto', marginBottom: 8 }}>
        {snippet}
      </pre>

      <button type="button" className="tab-btn" onClick={handleCopy}>
        {copied ? t('mcp.copied') : t('mcp.copy')}
      </button>
    </div>
  )
}
