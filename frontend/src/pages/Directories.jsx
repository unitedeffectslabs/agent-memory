import React, { useState, useEffect } from 'react'

export default function Directories() {
  const [dirs, setDirs] = useState([])
  const [confirmRemove, setConfirmRemove] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => {
    loadDirs()
    const interval = setInterval(loadDirs, 5000)
    return () => clearInterval(interval)
  }, [])

  async function loadDirs() {
    try {
      const list = await window.go.main.App.ListDirectories()
      setDirs(list || [])
    } catch (e) {
      console.error('Directories loadDirs error:', e)
    }
  }

  async function handleAdd() {
    setError('')
    try {
      const path = await window.go.main.App.SelectDirectory()
      if (!path) return
      await window.go.main.App.AddDirectory(path)
      await loadDirs()
    } catch (e) {
      setError('Failed to add directory: ' + (e?.message || String(e)))
    }
  }

  async function handleRemove(path) {
    if (confirmRemove !== path) {
      setConfirmRemove(path)
      return
    }
    setError('')
    try {
      await window.go.main.App.RemoveDirectory(path)
      setConfirmRemove(null)
      await loadDirs()
    } catch (e) {
      setError('Failed to remove directory: ' + (e?.message || String(e)))
    }
  }

  return (
    <div>
      <div style={s.header}>
        <h1 style={s.pageTitle}>Directories</h1>
        <button style={s.addBtn} onClick={handleAdd}>
          + Add Directory
        </button>
      </div>

      {error && <div style={s.error}>{error}</div>}

      <div style={s.list}>
        {dirs.length === 0 ? (
          <div style={s.empty}>
            <div style={{ fontSize: 28, marginBottom: 8, opacity: 0.3 }}>📁</div>
            <div>No directories being watched</div>
            <div style={{ fontSize: 12, color: 'rgba(255,255,255,0.3)', marginTop: 4 }}>
              Click "Add Directory" to get started
            </div>
          </div>
        ) : (
          dirs.map((dir) => (
            <div key={dir.Path} style={s.item}>
              <div style={s.itemInfo}>
                <div style={s.itemPath}>{dir.Path}</div>
                <div style={s.itemMeta}>
                  <span>{dir.FileCount ?? 0} files</span>
                  <span style={s.metaDot}>·</span>
                  <span>{dir.ChunkCount ?? 0} chunks</span>
                  {dir.Status && (
                    <>
                      <span style={s.metaDot}>·</span>
                      <span>{dir.Status}</span>
                    </>
                  )}
                </div>
              </div>
              <button
                style={{
                  ...s.removeBtn,
                  ...(confirmRemove === dir.Path ? s.removeBtnConfirm : {}),
                }}
                onClick={() => handleRemove(dir.Path)}
                onBlur={() => setConfirmRemove(null)}
              >
                {confirmRemove === dir.Path ? 'Confirm' : 'Remove'}
              </button>
            </div>
          ))
        )}
      </div>
    </div>
  )
}

const s = {
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 20,
  },
  pageTitle: {
    fontSize: 22,
    fontWeight: 700,
    color: '#fff',
    letterSpacing: '-0.3px',
  },
  addBtn: {
    padding: '7px 16px',
    background: '#0a84ff',
    color: '#fff',
    border: 'none',
    borderRadius: 6,
    fontSize: 13,
    cursor: 'pointer',
    fontWeight: 500,
  },
  list: {
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
  },
  item: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    background: 'rgba(255,255,255,0.04)',
    border: '1px solid rgba(255,255,255,0.06)',
    borderRadius: 10,
    padding: '14px 16px',
    transition: 'background 0.15s',
  },
  itemInfo: {
    flex: 1,
    minWidth: 0,
  },
  itemPath: {
    fontSize: 13,
    fontWeight: 500,
    color: '#e5e5e7',
    wordBreak: 'break-all',
    marginBottom: 4,
  },
  itemMeta: {
    fontSize: 12,
    color: 'rgba(255,255,255,0.35)',
    display: 'flex',
    gap: 6,
    alignItems: 'center',
  },
  metaDot: {
    opacity: 0.4,
  },
  removeBtn: {
    padding: '5px 12px',
    background: 'none',
    color: 'rgba(255,255,255,0.4)',
    border: '1px solid rgba(255,255,255,0.1)',
    borderRadius: 6,
    fontSize: 12,
    cursor: 'pointer',
    flexShrink: 0,
    marginLeft: 16,
    transition: 'all 0.15s',
  },
  removeBtnConfirm: {
    background: 'rgba(255,69,58,0.15)',
    color: '#ff6961',
    borderColor: 'rgba(255,69,58,0.3)',
  },
  empty: {
    color: 'rgba(255,255,255,0.4)',
    fontSize: 14,
    padding: '48px 32px',
    textAlign: 'center',
    background: 'rgba(255,255,255,0.02)',
    borderRadius: 12,
    border: '1px dashed rgba(255,255,255,0.08)',
  },
  error: {
    color: '#ff6961',
    fontSize: 13,
    padding: '10px 14px',
    background: 'rgba(255,69,58,0.08)',
    borderRadius: 8,
    marginBottom: 12,
  },
}
