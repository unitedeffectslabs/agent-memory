import React, { useState, useEffect } from 'react'

const MODELS = ['text-embedding-3-small', 'text-embedding-3-large']

export default function Settings() {
  const [apiKey, setApiKey] = useState('')
  const [model, setModel] = useState('')
  const [chunkSize, setChunkSize] = useState('')
  const [chunkOverlap, setChunkOverlap] = useState('')
  const [port, setPort] = useState('')
  const [authToken, setAuthToken] = useState('')
  const [claudeConfigPath, setClaudeConfigPath] = useState('')
  const [saved, setSaved] = useState({})
  const [error, setError] = useState('')
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [pendingModel, setPendingModel] = useState(null) // model change awaiting confirmation

  useEffect(() => { loadSettings() }, [])

  async function loadSettings() {
    try {
      const [key, m, cs, co, p, token] = await Promise.all([
        window.go.main.App.GetConfig('openai_api_key'),
        window.go.main.App.GetConfig('embedding_model'),
        window.go.main.App.GetConfig('chunk_size'),
        window.go.main.App.GetConfig('chunk_overlap'),
        window.go.main.App.GetConfig('mcp_port'),
        window.go.main.App.GetConfig('auth_token'),
      ])
      setApiKey(key || '')
      setModel(m || 'text-embedding-3-small')
      setChunkSize(cs || '512')
      setChunkOverlap(co || '50')
      setPort(p || '9847')
      setAuthToken(token || '')
      const cPath = await window.go.main.App.GetClaudeDesktopConfigPath()
      setClaudeConfigPath(cPath || '')
    } catch {
      // Backend may not be ready
    }
  }

  async function saveConfig(key, value, label) {
    setError('')
    try {
      await window.go.main.App.SetConfig(key, value)
      setSaved((prev) => ({ ...prev, [label]: true }))
      setTimeout(() => setSaved((prev) => ({ ...prev, [label]: false })), 2000)
    } catch (e) {
      setError('Failed to save ' + label + ': ' + (e?.message || String(e)))
    }
  }

  async function handleRotateToken() {
    setError('')
    try {
      const token = await window.go.main.App.RotateAuthToken()
      setAuthToken(token)
      setSaved((prev) => ({ ...prev, token: true }))
      setTimeout(() => setSaved((prev) => ({ ...prev, token: false })), 2000)
    } catch (e) {
      setError('Failed to rotate token: ' + (e?.message || String(e)))
    }
  }

  async function handleInstallClaude() {
    setError('')
    try {
      await window.go.main.App.InstallToClaudeDesktop()
      setSaved((prev) => ({ ...prev, claudeInstall: true }))
      setTimeout(() => setSaved((prev) => ({ ...prev, claudeInstall: false })), 4000)
    } catch (e) {
      setError('Failed to install: ' + (e?.message || String(e)))
    }
  }

  async function handleSaveClaudePath() {
    setError('')
    try {
      await window.go.main.App.SetClaudeDesktopConfigPath(claudeConfigPath)
      setSaved((prev) => ({ ...prev, claudePath: true }))
      setTimeout(() => setSaved((prev) => ({ ...prev, claudePath: false })), 2000)
    } catch (e) {
      setError('Failed to save path: ' + (e?.message || String(e)))
    }
  }

  return (
    <div>
      <h1 style={s.pageTitle}>Settings</h1>

      {error && <div style={s.error}>{error}</div>}

      {/* API Configuration */}
      <Section title="API Configuration">
        <Row label="OpenAI API Key">
          <input
            type="password"
            style={s.input}
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder="sk-..."
          />
          <SaveBtn onClick={() => saveConfig('openai_api_key', apiKey, 'apiKey')} saved={saved.apiKey} />
        </Row>
        <Row label="Embedding Model">
          <select
            style={s.select}
            value={model}
            onChange={async (e) => {
              const newModel = e.target.value
              if (newModel === model) return
              // Check if there are existing embeddings that would be wiped
              try {
                const stats = await window.go.main.App.GetStats()
                if (stats.TotalChunks > 0) {
                  setPendingModel(newModel) // show confirmation dialog
                  return
                }
              } catch { /* proceed anyway */ }
              setModel(newModel)
              saveConfig('embedding_model', newModel, 'model')
            }}
          >
            {MODELS.map((m) => <option key={m} value={m}>{m}</option>)}
          </select>
          {saved.model && <span style={s.saved}>Saved</span>}
        </Row>
      </Section>

      {/* Chunking */}
      <Section title="Chunking">
        <Row label="Chunk Size (tokens)">
          <input
            type="number"
            style={{ ...s.input, maxWidth: 100 }}
            value={chunkSize}
            onChange={(e) => setChunkSize(e.target.value)}
            min="64" max="8192"
          />
          <SaveBtn onClick={() => saveConfig('chunk_size', chunkSize, 'chunkSize')} saved={saved.chunkSize} />
        </Row>
        <Row label="Chunk Overlap (tokens)">
          <input
            type="number"
            style={{ ...s.input, maxWidth: 100 }}
            value={chunkOverlap}
            onChange={(e) => setChunkOverlap(e.target.value)}
            min="0" max="1024"
          />
          <SaveBtn onClick={() => saveConfig('chunk_overlap', chunkOverlap, 'chunkOverlap')} saved={saved.chunkOverlap} />
        </Row>
      </Section>

      {/* MCP Server */}
      <Section title="MCP Server">
        <Row label="Port">
          <input
            type="number"
            style={{ ...s.input, maxWidth: 100 }}
            value={port}
            onChange={(e) => setPort(e.target.value)}
            min="1024" max="65535"
          />
          <SaveBtn onClick={() => saveConfig('mcp_port', port, 'port')} saved={saved.port} />
        </Row>
        <Row label="Auth Token">
          <div style={s.tokenBox}>{authToken || '-'}</div>
          <button style={s.btn} onClick={handleRotateToken}>Rotate</button>
          {saved.token && <span style={s.saved}>Rotated</span>}
        </Row>
      </Section>

      {/* Network Info */}
      <Section title="Network">
        <Row label="MCP Endpoint">
          <span style={s.readOnly}>127.0.0.1:{port || '9847'}</span>
        </Row>
        <Row label="Outbound">
          <span style={s.readOnly}>api.openai.com only</span>
        </Row>
      </Section>

      {/* Claude Desktop */}
      <Section title="Claude Desktop">
        <Row label="Config Path">
          <input
            type="text"
            style={s.input}
            value={claudeConfigPath}
            onChange={(e) => setClaudeConfigPath(e.target.value)}
          />
          <SaveBtn onClick={handleSaveClaudePath} saved={saved.claudePath} />
        </Row>
        <Row label="Install">
          <button style={{ ...s.btn, ...s.btnPrimary }} onClick={handleInstallClaude}>
            Install to Claude Desktop
          </button>
          {saved.claudeInstall && <span style={s.saved}>Installed — restart Claude Desktop to activate</span>}
        </Row>
      </Section>

      {/* Model change confirmation dialog */}
      {pendingModel && (
        <div style={s.overlay} onClick={() => setPendingModel(null)}>
          <div style={s.dialog} onClick={(e) => e.stopPropagation()}>
            <div style={s.dialogTitle}>Change Embedding Model?</div>
            <div style={s.dialogText}>
              Switching from <strong>{model}</strong> to <strong>{pendingModel}</strong> will
              reset the index and re-embed all files. Existing embeddings will be deleted because
              vectors from different models are incompatible.
            </div>
            <div style={s.dialogButtons}>
              <button style={s.btn} onClick={() => setPendingModel(null)}>Cancel</button>
              <button
                style={{ ...s.btn, background: '#0a84ff', color: '#fff', borderColor: 'transparent' }}
                onClick={() => {
                  const newModel = pendingModel
                  setPendingModel(null)
                  setModel(newModel)
                  saveConfig('embedding_model', newModel, 'model')
                }}
              >
                Change &amp; Reset Index
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Danger Zone */}
      <div style={s.danger}>
        <div style={s.dangerTitle}>Danger Zone</div>
        <p style={s.dangerText}>
          Delete the local database and quit the app. This permanently removes all indexed data,
          embeddings, watched directories, and configuration. To fully uninstall, delete the app
          after this step. If you reopen the app, a fresh database will be created.
        </p>
        {!showDeleteConfirm ? (
          <button style={s.dangerBtn} onClick={() => setShowDeleteConfirm(true)}>
            Delete Database &amp; Quit
          </button>
        ) : (
          <div style={s.dangerConfirm}>
            <p style={{ fontSize: 13, color: '#e5e5e7', marginBottom: 14, lineHeight: 1.5 }}>
              <strong style={{ color: '#ff453a' }}>Are you sure?</strong>{' '}
              This will physically delete the database file. All data will be lost.
              The app will quit immediately.
            </p>
            <div style={{ display: 'flex', gap: 8 }}>
              <button
                style={{ ...s.btn, background: '#ff453a', color: '#fff', borderColor: 'transparent' }}
                onClick={async () => {
                  try {
                    await window.go.main.App.DeleteDatabaseAndQuit()
                  } catch (e) {
                    setError('Failed: ' + (e?.message || String(e)))
                    setShowDeleteConfirm(false)
                  }
                }}
              >
                Yes, Delete &amp; Quit
              </button>
              <button style={s.btn} onClick={() => setShowDeleteConfirm(false)}>Cancel</button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function Section({ title, children }) {
  return (
    <div style={s.section}>
      <div style={s.sectionTitle}>{title}</div>
      <div style={s.sectionContent}>{children}</div>
    </div>
  )
}

function Row({ label, children }) {
  return (
    <div style={s.row}>
      <div style={s.label}>{label}</div>
      <div style={s.rowControls}>{children}</div>
    </div>
  )
}

function SaveBtn({ onClick, saved }) {
  return (
    <>
      <button style={{ ...s.btn, ...s.btnPrimary }} onClick={onClick}>Save</button>
      {saved && <span style={s.saved}>Saved</span>}
    </>
  )
}

const s = {
  pageTitle: {
    fontSize: 22,
    fontWeight: 700,
    color: '#fff',
    marginBottom: 24,
    letterSpacing: '-0.3px',
  },
  section: {
    marginBottom: 28,
  },
  sectionTitle: {
    fontSize: 11,
    fontWeight: 600,
    color: 'rgba(255,255,255,0.35)',
    textTransform: 'uppercase',
    letterSpacing: '0.8px',
    marginBottom: 10,
  },
  sectionContent: {
    background: 'rgba(255,255,255,0.04)',
    border: '1px solid rgba(255,255,255,0.06)',
    borderRadius: 10,
    overflow: 'hidden',
  },
  row: {
    display: 'flex',
    alignItems: 'center',
    padding: '12px 16px',
    borderBottom: '1px solid rgba(255,255,255,0.04)',
  },
  label: {
    fontSize: 13,
    color: 'rgba(255,255,255,0.6)',
    width: 160,
    flexShrink: 0,
  },
  rowControls: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    flex: 1,
    minWidth: 0,
  },
  input: {
    flex: 1,
    maxWidth: 280,
    padding: '6px 10px',
    background: 'rgba(255,255,255,0.06)',
    border: '1px solid rgba(255,255,255,0.1)',
    borderRadius: 6,
    color: '#e5e5e7',
    fontSize: 13,
    outline: 'none',
  },
  select: {
    flex: 1,
    maxWidth: 280,
    padding: '6px 10px',
    background: 'rgba(255,255,255,0.06)',
    border: '1px solid rgba(255,255,255,0.1)',
    borderRadius: 6,
    color: '#e5e5e7',
    fontSize: 13,
    outline: 'none',
    cursor: 'pointer',
  },
  tokenBox: {
    flex: 1,
    maxWidth: 280,
    padding: '6px 10px',
    background: 'rgba(255,255,255,0.04)',
    border: '1px solid rgba(255,255,255,0.08)',
    borderRadius: 6,
    color: 'rgba(255,255,255,0.5)',
    fontSize: 11,
    fontFamily: 'SF Mono, Menlo, monospace',
    wordBreak: 'break-all',
    userSelect: 'all',
  },
  btn: {
    padding: '6px 14px',
    background: 'rgba(255,255,255,0.08)',
    color: 'rgba(255,255,255,0.7)',
    border: '1px solid rgba(255,255,255,0.1)',
    borderRadius: 6,
    fontSize: 13,
    cursor: 'pointer',
    fontWeight: 500,
    flexShrink: 0,
    transition: 'all 0.15s',
  },
  btnPrimary: {
    background: '#0a84ff',
    color: '#fff',
    borderColor: 'transparent',
  },
  readOnly: {
    fontSize: 13,
    color: '#0a84ff',
  },
  saved: {
    fontSize: 12,
    color: '#34c759',
    fontWeight: 500,
  },
  error: {
    color: '#ff6961',
    fontSize: 13,
    padding: '10px 14px',
    background: 'rgba(255,69,58,0.08)',
    borderRadius: 8,
    marginBottom: 16,
  },
  danger: {
    marginTop: 40,
    paddingTop: 24,
    borderTop: '1px solid rgba(255,69,58,0.15)',
  },
  dangerTitle: {
    fontSize: 11,
    fontWeight: 600,
    color: '#ff453a',
    textTransform: 'uppercase',
    letterSpacing: '0.8px',
    marginBottom: 10,
  },
  dangerText: {
    fontSize: 13,
    color: 'rgba(255,255,255,0.45)',
    lineHeight: 1.6,
    marginBottom: 16,
  },
  dangerBtn: {
    padding: '7px 16px',
    background: 'none',
    color: '#ff453a',
    border: '1px solid rgba(255,69,58,0.3)',
    borderRadius: 6,
    fontSize: 13,
    cursor: 'pointer',
    fontWeight: 500,
  },
  dangerConfirm: {
    background: 'rgba(255,69,58,0.06)',
    border: '1px solid rgba(255,69,58,0.2)',
    borderRadius: 10,
    padding: 16,
  },
  overlay: {
    position: 'fixed',
    inset: 0,
    background: 'rgba(0,0,0,0.5)',
    backdropFilter: 'blur(4px)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 1000,
  },
  dialog: {
    background: '#2a2a2c',
    border: '1px solid rgba(255,255,255,0.1)',
    borderRadius: 12,
    padding: 24,
    maxWidth: 400,
    width: '90%',
    boxShadow: '0 20px 60px rgba(0,0,0,0.5)',
  },
  dialogTitle: {
    fontSize: 16,
    fontWeight: 600,
    color: '#fff',
    marginBottom: 8,
  },
  dialogText: {
    fontSize: 13,
    color: 'rgba(255,255,255,0.55)',
    lineHeight: 1.5,
    marginBottom: 20,
  },
  dialogButtons: {
    display: 'flex',
    gap: 8,
    justifyContent: 'flex-end',
  },
}
