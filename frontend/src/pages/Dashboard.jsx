import React, { useState, useEffect } from 'react'
import Controls from '../components/Controls'

export default function Dashboard() {
  const [stats, setStats] = useState(null)
  const [running, setRunning] = useState(false)

  useEffect(() => {
    loadData()
    const interval = setInterval(loadData, 2000)
    return () => clearInterval(interval)
  }, [])

  async function loadData() {
    try {
      const [s, r] = await Promise.all([
        window.go.main.App.GetStats(),
        window.go.main.App.IsRunning(),
      ])
      setStats(s)
      setRunning(r)
    } catch (e) {
      console.error('Dashboard loadData error:', e)
    }
  }

  function formatTime(ts) {
    if (!ts) return 'Never'
    const d = new Date(ts)
    if (isNaN(d.getTime()) || d.getFullYear() < 2000) return 'Never'
    return d.toLocaleString()
  }

  const isIndexing = stats?.IsIndexing
  const progress = isIndexing && stats?.TotalToIndex > 0
    ? Math.round((stats.IndexedFiles / stats.TotalToIndex) * 100)
    : null

  return (
    <div>
      <style>{`
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.4; }
        }
        @keyframes progressBar {
          0% { background-position: 200% 0; }
          100% { background-position: -200% 0; }
        }
      `}</style>

      <h1 style={st.pageTitle}>Dashboard</h1>

      {/* Status badge */}
      <div style={{ marginBottom: 24 }}>
        {isIndexing ? (
          <div>
            <div style={st.badge}>
              <div style={{ ...st.dot, background: '#34c759', animation: 'pulse 1.5s ease-in-out infinite' }} />
              <span>
                Indexing{progress !== null ? ` — ${stats.IndexedFiles} / ${stats.TotalToIndex} files (${progress}%)` : '...'}
              </span>
            </div>
            {progress !== null && (
              <div style={st.progressTrack}>
                <div style={{ ...st.progressBar, width: `${progress}%` }} />
              </div>
            )}
          </div>
        ) : running ? (
          <div style={st.badge}>
            <div style={{ ...st.dot, background: '#64a0ff' }} />
            <span>Watching for changes</span>
          </div>
        ) : (
          <div style={{ ...st.badge, color: 'rgba(255,255,255,0.35)' }}>
            <div style={{ ...st.dot, background: 'rgba(255,255,255,0.2)' }} />
            Stopped
          </div>
        )}
      </div>

      {/* Stats grid */}
      <div style={st.grid}>
        <StatCard label="Total Files" value={stats?.TotalFiles ?? '-'} />
        <StatCard label="Total Chunks" value={stats?.TotalChunks ?? '-'} />
        <StatCard label="Embedding Model" value={stats?.EmbeddingModel || '-'} small />
        <StatCard label="Last Indexed" value={formatTime(stats?.LastIndexedAt)} small />
      </div>

      {/* Controls */}
      <div style={{ marginTop: 28 }}>
        <div style={st.sectionLabel}>Controls</div>
        <Controls onAction={loadData} />
      </div>
    </div>
  )
}

function StatCard({ label, value, small }) {
  return (
    <div style={st.card}>
      <div style={st.cardLabel}>{label}</div>
      <div style={{ ...st.cardValue, ...(small ? { fontSize: 14 } : {}) }}>{value}</div>
    </div>
  )
}

const st = {
  pageTitle: {
    fontSize: 22,
    fontWeight: 700,
    color: '#fff',
    marginBottom: 20,
    letterSpacing: '-0.3px',
  },
  badge: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 8,
    padding: '6px 14px',
    background: 'rgba(255,255,255,0.06)',
    borderRadius: 20,
    fontSize: 13,
    color: 'rgba(255,255,255,0.7)',
  },
  dot: {
    width: 8,
    height: 8,
    borderRadius: '50%',
  },
  progressTrack: {
    marginTop: 10,
    height: 4,
    background: 'rgba(255,255,255,0.06)',
    borderRadius: 2,
    overflow: 'hidden',
  },
  progressBar: {
    height: '100%',
    background: '#34c759',
    borderRadius: 2,
    transition: 'width 0.5s ease',
  },
  grid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))',
    gap: 12,
  },
  card: {
    background: 'rgba(255,255,255,0.04)',
    border: '1px solid rgba(255,255,255,0.06)',
    borderRadius: 10,
    padding: '16px 18px',
  },
  cardLabel: {
    fontSize: 11,
    fontWeight: 500,
    color: 'rgba(255,255,255,0.4)',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
    marginBottom: 6,
  },
  cardValue: {
    fontSize: 22,
    fontWeight: 600,
    color: '#fff',
    letterSpacing: '-0.3px',
  },
  sectionLabel: {
    fontSize: 11,
    fontWeight: 600,
    color: 'rgba(255,255,255,0.35)',
    textTransform: 'uppercase',
    letterSpacing: '0.8px',
    marginBottom: 12,
  },
}
