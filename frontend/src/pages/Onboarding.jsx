import React, { useState } from 'react'

export default function Onboarding({ onComplete }) {
  const [step, setStep] = useState(0)
  const [apiKey, setApiKey] = useState('')
  const [dirs, setDirs] = useState([])
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  async function handleSaveApiKey() {
    if (!apiKey.trim()) { setError('API key is required.'); return }
    setSaving(true); setError('')
    try {
      await window.go.main.App.SetConfig('openai_api_key', apiKey.trim())
      setStep(2)
    } catch (e) {
      setError('Failed to save: ' + (e?.message || String(e)))
    } finally { setSaving(false) }
  }

  async function handleChooseFolder() {
    setError('')
    try {
      const path = await window.go.main.App.SelectDirectory()
      if (!path) return
      if (path === '/') { setError('Cannot watch the root directory.'); return }
      if (dirs.includes(path)) { setError('Directory already added.'); return }
      setDirs((prev) => [...prev, path])
    } catch (e) {
      setError('Failed to open folder picker: ' + (e?.message || String(e)))
    }
  }

  function handleRemoveDirectory(path) {
    setDirs((prev) => prev.filter((d) => d !== path))
  }

  async function handleSaveDirsAndContinue() {
    setError('')
    try {
      for (const path of dirs) {
        await window.go.main.App.RegisterDirectory(path)
      }
      setStep(3)
    } catch (e) {
      setError('Failed to save directories: ' + (e?.message || String(e)))
    }
  }

  function handleFinish() {
    onComplete()
  }

  const totalSteps = 4

  return (
    <div style={s.container}>
      <div style={s.card}>
        {/* Step dots */}
        <div style={s.dots}>
          {Array.from({ length: totalSteps }).map((_, i) => (
            <div key={i} style={{ ...s.dot, ...(i <= step ? s.dotActive : {}) }} />
          ))}
        </div>

        {step === 0 && (
          <>
            <h2 style={s.heading}>Welcome to Agent Memory</h2>
            <p style={s.text}>
              Agent Memory watches your directories, creates embeddings with OpenAI,
              and stores vectors locally. It provides semantic search via MCP for
              Claude and other AI assistants.
            </p>
            <div style={s.btnRow}>
              <button style={s.btnPrimary} onClick={() => setStep(1)}>Get Started</button>
            </div>
          </>
        )}

        {step === 1 && (
          <>
            <h2 style={s.heading}>OpenAI API Key</h2>
            <p style={s.text}>
              Enter your OpenAI API key to enable embeddings.
              It's stored locally in the database — never sent anywhere except OpenAI.
            </p>
            <input
              type="password"
              style={s.input}
              placeholder="sk-..."
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSaveApiKey()}
            />
            {error && <div style={s.error}>{error}</div>}
            <div style={s.btnRow}>
              <button style={s.btn} onClick={() => setStep(0)}>Back</button>
              <button style={s.btnPrimary} onClick={handleSaveApiKey} disabled={saving}>
                {saving ? 'Saving...' : 'Continue'}
              </button>
            </div>
          </>
        )}

        {step === 2 && (
          <>
            <h2 style={s.heading}>Add Directories</h2>
            <p style={s.text}>
              Choose one or more directories to watch. All supported files will be
              indexed and kept up to date as they change.
            </p>

            {/* Directory list — always visible */}
            <div style={s.dirList}>
              {dirs.length === 0 ? (
                <div style={s.dirEmpty}>No directories selected yet</div>
              ) : (
                dirs.map((path) => (
                  <div key={path} style={s.dirItem}>
                    <span style={s.dirPath}>{path}</span>
                    <button
                      style={s.dirRemove}
                      onClick={() => handleRemoveDirectory(path)}
                      title="Remove"
                    >
                      ✕
                    </button>
                  </div>
                ))
              )}
            </div>

            <button style={s.btn} onClick={handleChooseFolder}>
              {dirs.length === 0 ? 'Choose Folder' : '+ Add Another'}
            </button>

            {error && <div style={{ ...s.error, marginTop: 12 }}>{error}</div>}

            <div style={{ ...s.btnRow, marginTop: 20 }}>
              <button style={s.btn} onClick={() => setStep(1)}>Back</button>
              <button
                style={{ ...s.btnPrimary, ...(dirs.length === 0 ? s.btnDisabled : {}) }}
                onClick={handleSaveDirsAndContinue}
                disabled={dirs.length === 0}
              >
                Continue
              </button>
            </div>
          </>
        )}

        {step === 3 && (
          <>
            <h2 style={s.heading}>All Set</h2>
            <p style={s.text}>
              Agent Memory is configured. Before starting, you may want to review
              your embedding model in Settings. When you're ready, hit Start on
              the Dashboard to begin indexing.
            </p>
            <div style={s.btnRow}>
              <button style={s.btnPrimary} onClick={handleFinish}>Open Dashboard</button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

const s = {
  container: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    flex: 1,
    padding: 32,
    minHeight: '100vh',
  },
  card: {
    maxWidth: 440,
    width: '100%',
    background: 'rgba(255,255,255,0.04)',
    border: '1px solid rgba(255,255,255,0.08)',
    borderRadius: 14,
    padding: 32,
  },
  dots: {
    display: 'flex',
    gap: 6,
    marginBottom: 24,
    justifyContent: 'center',
  },
  dot: {
    width: 8,
    height: 8,
    borderRadius: '50%',
    background: 'rgba(255,255,255,0.12)',
    transition: 'background 0.2s',
  },
  dotActive: {
    background: '#0a84ff',
  },
  heading: {
    fontSize: 20,
    fontWeight: 700,
    color: '#fff',
    marginBottom: 10,
    letterSpacing: '-0.3px',
  },
  text: {
    fontSize: 14,
    color: 'rgba(255,255,255,0.5)',
    lineHeight: 1.6,
    marginBottom: 24,
  },
  input: {
    width: '100%',
    padding: '10px 12px',
    background: 'rgba(255,255,255,0.06)',
    border: '1px solid rgba(255,255,255,0.1)',
    borderRadius: 8,
    color: '#e5e5e7',
    fontSize: 14,
    marginBottom: 16,
    outline: 'none',
  },
  dirList: {
    display: 'flex',
    flexDirection: 'column',
    gap: 6,
    marginBottom: 14,
    padding: 12,
    background: 'rgba(255,255,255,0.03)',
    border: '1px solid rgba(255,255,255,0.08)',
    borderRadius: 10,
    minHeight: 60,
  },
  dirEmpty: {
    fontSize: 13,
    color: 'rgba(255,255,255,0.2)',
    textAlign: 'center',
    padding: '12px 0',
  },
  dirItem: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '8px 12px',
    background: 'rgba(255,255,255,0.04)',
    border: '1px solid rgba(255,255,255,0.08)',
    borderRadius: 8,
  },
  dirPath: {
    fontSize: 13,
    color: '#0a84ff',
    wordBreak: 'break-all',
    flex: 1,
    marginRight: 8,
  },
  dirRemove: {
    background: 'none',
    border: 'none',
    color: 'rgba(255,255,255,0.3)',
    fontSize: 14,
    cursor: 'pointer',
    padding: '2px 6px',
    borderRadius: 4,
    flexShrink: 0,
  },
  btnRow: {
    display: 'flex',
    gap: 10,
    justifyContent: 'flex-end',
  },
  btn: {
    padding: '8px 18px',
    background: 'rgba(255,255,255,0.08)',
    color: 'rgba(255,255,255,0.7)',
    border: '1px solid rgba(255,255,255,0.1)',
    borderRadius: 8,
    fontSize: 14,
    cursor: 'pointer',
    fontWeight: 500,
  },
  btnPrimary: {
    padding: '8px 22px',
    background: '#0a84ff',
    color: '#fff',
    border: 'none',
    borderRadius: 8,
    fontSize: 14,
    cursor: 'pointer',
    fontWeight: 600,
  },
  btnDisabled: {
    opacity: 0.35,
    cursor: 'default',
  },
  error: {
    color: '#ff6961',
    fontSize: 13,
    marginBottom: 12,
  },
}
