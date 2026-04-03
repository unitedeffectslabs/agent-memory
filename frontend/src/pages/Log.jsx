import React, { useState, useEffect, useRef } from 'react'

const PAGE_SIZE = 50

export default function Log() {
  const [entries, setEntries] = useState([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const listRef = useRef(null)

  useEffect(() => {
    loadEntries(true)
    const interval = setInterval(() => loadEntries(true), 3000)
    return () => clearInterval(interval)
  }, [])

  async function loadEntries(refresh) {
    try {
      const offset = refresh ? 0 : entries.length
      const res = await window.go.main.App.GetActivityLog(PAGE_SIZE, offset)
      if (!res) return
      if (refresh) {
        setEntries(res.Entries || [])
      } else {
        setEntries(prev => [...prev, ...(res.Entries || [])])
      }
      setTotal(res.Total || 0)
    } catch (e) {
      console.error('Log loadEntries error:', e)
    }
  }

  async function handleLoadMore() {
    setLoading(true)
    await loadEntries(false)
    setLoading(false)
  }

  function formatTime(ts) {
    if (!ts) return ''
    const d = new Date(ts)
    if (isNaN(d.getTime()) || d.getFullYear() < 2000) return ''
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) +
      ' ' + d.toLocaleDateString([], { month: 'short', day: 'numeric' })
  }

  function shortenPath(path) {
    if (!path) return ''
    const parts = path.split('/')
    if (parts.length <= 4) return path
    return '.../' + parts.slice(-3).join('/')
  }

  const actionColors = {
    indexed: { bg: 'rgba(52,199,89,0.12)', color: '#34c759' },
    ignored: { bg: 'rgba(255,159,10,0.12)', color: '#ff9f0a' },
    deleted: { bg: 'rgba(255,69,58,0.12)', color: '#ff453a' },
    error:   { bg: 'rgba(255,69,58,0.25)', color: '#ff6961' },
  }

  return (
    <div>
      <div style={s.header}>
        <h1 style={s.pageTitle}>Activity Log</h1>
        <span style={s.countBadge}>{total} events</span>
      </div>

      <div ref={listRef} style={s.list}>
        {entries.length === 0 ? (
          <div style={s.empty}>
            <div style={{ fontSize: 28, marginBottom: 8, opacity: 0.3 }}>~</div>
            <div>No activity yet</div>
            <div style={{ fontSize: 12, color: 'rgba(255,255,255,0.3)', marginTop: 4 }}>
              Start the engine to begin indexing files
            </div>
          </div>
        ) : (
          entries.map((entry, idx) => {
            const ac = actionColors[entry.Action] || actionColors.error
            return (
              <div key={entry.ID || idx} style={s.row}>
                <div style={s.rowLeft}>
                  <span style={{
                    ...s.badge,
                    background: ac.bg,
                    color: ac.color,
                  }}>
                    {entry.Action}
                  </span>
                  <span style={s.path} title={entry.Path}>
                    {shortenPath(entry.Path)}
                  </span>
                </div>
                <div style={s.rowRight}>
                  {entry.Detail && (
                    <span style={s.detail}>{entry.Detail}</span>
                  )}
                  <span style={s.time}>{formatTime(entry.Timestamp)}</span>
                </div>
              </div>
            )
          })
        )}
      </div>

      {entries.length < total && (
        <button
          style={s.loadMore}
          onClick={handleLoadMore}
          disabled={loading}
        >
          {loading ? 'Loading...' : `Load more (${entries.length} of ${total})`}
        </button>
      )}
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
  countBadge: {
    fontSize: 12,
    color: 'rgba(255,255,255,0.4)',
    background: 'rgba(255,255,255,0.06)',
    padding: '4px 10px',
    borderRadius: 12,
  },
  list: {
    display: 'flex',
    flexDirection: 'column',
    gap: 2,
  },
  row: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '8px 12px',
    background: 'rgba(255,255,255,0.02)',
    borderRadius: 6,
    fontSize: 12,
    gap: 12,
    minHeight: 34,
  },
  rowLeft: {
    display: 'flex',
    alignItems: 'center',
    gap: 10,
    flex: 1,
    minWidth: 0,
  },
  rowRight: {
    display: 'flex',
    alignItems: 'center',
    gap: 12,
    flexShrink: 0,
  },
  badge: {
    padding: '2px 8px',
    borderRadius: 4,
    fontSize: 11,
    fontWeight: 600,
    textTransform: 'uppercase',
    letterSpacing: '0.3px',
    flexShrink: 0,
    minWidth: 58,
    textAlign: 'center',
  },
  path: {
    color: 'rgba(255,255,255,0.65)',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  detail: {
    color: 'rgba(255,255,255,0.35)',
    fontSize: 11,
    whiteSpace: 'nowrap',
  },
  time: {
    color: 'rgba(255,255,255,0.25)',
    fontSize: 11,
    whiteSpace: 'nowrap',
    minWidth: 100,
    textAlign: 'right',
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
  loadMore: {
    marginTop: 12,
    padding: '8px 16px',
    background: 'rgba(255,255,255,0.06)',
    color: 'rgba(255,255,255,0.5)',
    border: '1px solid rgba(255,255,255,0.08)',
    borderRadius: 6,
    fontSize: 12,
    cursor: 'pointer',
    width: '100%',
    textAlign: 'center',
  },
}
