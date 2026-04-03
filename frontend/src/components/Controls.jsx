import React, { useState } from 'react'

export default function Controls({ onAction }) {
  const [loading, setLoading] = useState(null)
  const [showResetConfirm, setShowResetConfirm] = useState(false)

  async function handleAction(action, name) {
    setLoading(name)
    try {
      await action()
      if (onAction) onAction()
    } catch {
      // Errors handled silently for controls
    } finally {
      setLoading(null)
    }
  }

  async function handleReset() {
    setShowResetConfirm(false)
    setLoading('reset')
    try {
      await window.go.main.App.Reset()
      if (onAction) onAction()
    } catch {
      // Errors handled silently
    } finally {
      setLoading(null)
    }
  }

  return (
    <>
      <div style={s.row}>
        <button
          style={{ ...s.btn, ...s.btnGreen }}
          onClick={() => handleAction(() => window.go.main.App.Start(), 'start')}
          disabled={loading !== null}
        >
          {loading === 'start' ? 'Starting...' : 'Start'}
        </button>
        <button
          style={{ ...s.btn, ...s.btnRed }}
          onClick={() => handleAction(() => window.go.main.App.Stop(), 'stop')}
          disabled={loading !== null}
        >
          {loading === 'stop' ? 'Stopping...' : 'Stop'}
        </button>
        <button
          style={s.btn}
          onClick={() => handleAction(() => window.go.main.App.Restart(), 'restart')}
          disabled={loading !== null}
        >
          {loading === 'restart' ? 'Restarting...' : 'Restart'}
        </button>
        <button
          style={{ ...s.btn, ...s.btnRed }}
          onClick={() => setShowResetConfirm(true)}
          disabled={loading !== null}
        >
          {loading === 'reset' ? 'Resetting...' : 'Reset Index'}
        </button>
      </div>

      {showResetConfirm && (
        <div style={s.overlay} onClick={() => setShowResetConfirm(false)}>
          <div style={s.dialog} onClick={(e) => e.stopPropagation()}>
            <div style={s.dialogTitle}>Reset Index</div>
            <div style={s.dialogText}>
              This will clear all embeddings and re-index everything from scratch.
              This cannot be undone.
            </div>
            <div style={s.dialogButtons}>
              <button style={s.btn} onClick={() => setShowResetConfirm(false)}>
                Cancel
              </button>
              <button style={{ ...s.btn, background: '#ff453a', color: '#fff' }} onClick={handleReset}>
                Reset Everything
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}

const s = {
  row: {
    display: 'flex',
    gap: 8,
    flexWrap: 'wrap',
  },
  btn: {
    padding: '7px 16px',
    background: 'rgba(255,255,255,0.08)',
    color: 'rgba(255,255,255,0.75)',
    border: '1px solid rgba(255,255,255,0.1)',
    borderRadius: 6,
    fontSize: 13,
    cursor: 'pointer',
    fontWeight: 500,
    transition: 'all 0.15s',
  },
  btnGreen: {
    background: 'rgba(52,199,89,0.15)',
    color: '#34c759',
    borderColor: 'rgba(52,199,89,0.25)',
  },
  btnRed: {
    background: 'rgba(255,69,58,0.1)',
    color: '#ff6961',
    borderColor: 'rgba(255,69,58,0.2)',
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
    maxWidth: 380,
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
