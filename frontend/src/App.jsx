import React, { useState, useEffect } from 'react'
import Onboarding from './pages/Onboarding'
import Dashboard from './pages/Dashboard'
import Directories from './pages/Directories'
import Log from './pages/Log'
import Settings from './pages/Settings'

const tabs = [
  { id: 'Dashboard', icon: '⊞', label: 'Dashboard' },
  { id: 'Directories', icon: '⊘', label: 'Directories' },
  { id: 'Log', icon: '⊙', label: 'Log' },
  { id: 'Settings', icon: '⊛', label: 'Settings' },
]

export default function App() {
  const [activeTab, setActiveTab] = useState('Dashboard')
  const [needsOnboarding, setNeedsOnboarding] = useState(null)

  useEffect(() => {
    checkOnboarding()
  }, [])

  async function checkOnboarding() {
    try {
      const apiKey = await window.go.main.App.GetConfig('openai_api_key')
      setNeedsOnboarding(!apiKey || apiKey.trim() === '')
    } catch {
      setNeedsOnboarding(true)
    }
  }

  function handleOnboardingComplete() {
    setNeedsOnboarding(false)
    setActiveTab('Dashboard')
  }

  if (needsOnboarding === null) {
    return (
      <div style={styles.shell}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', flex: 1 }}>
          <span style={{ color: 'rgba(255,255,255,0.3)', fontSize: 13 }}>Loading...</span>
        </div>
      </div>
    )
  }

  if (needsOnboarding) {
    return (
      <div style={styles.shell}>
        <Onboarding onComplete={handleOnboardingComplete} />
      </div>
    )
  }

  return (
    <div style={styles.shell}>
      {/* Sidebar */}
      <div style={styles.sidebar}>
        <div style={styles.sidebarHeader}>
          <span style={styles.logoText}>Agent Memory</span>
        </div>
        <nav style={styles.nav}>
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              style={{
                ...styles.navItem,
                ...(activeTab === tab.id ? styles.navItemActive : {}),
              }}
            >
              <span style={styles.navIcon}>{tab.icon}</span>
              <span>{tab.label}</span>
            </button>
          ))}
        </nav>
      </div>

      {/* Main content */}
      <div style={styles.main}>
        <div style={styles.titleBar} />
        <div style={styles.content}>
          {activeTab === 'Dashboard' && <Dashboard />}
          {activeTab === 'Directories' && <Directories />}
          {activeTab === 'Log' && <Log />}
          {activeTab === 'Settings' && <Settings />}
        </div>
      </div>
    </div>
  )
}

const styles = {
  shell: {
    display: 'flex',
    height: '100vh',
    background: '#1e1e1e',
  },
  sidebar: {
    width: 200,
    background: 'rgba(30,30,30,0.85)',
    borderRight: '1px solid rgba(255,255,255,0.08)',
    display: 'flex',
    flexDirection: 'column',
    flexShrink: 0,
    WebkitAppRegion: 'drag',
  },
  sidebarHeader: {
    padding: '20px 16px 8px',
    display: 'flex',
    alignItems: 'center',
    gap: 8,
  },
  logoText: {
    fontSize: 13,
    fontWeight: 700,
    color: 'rgba(255,255,255,0.85)',
    letterSpacing: '-0.2px',
  },
  nav: {
    display: 'flex',
    flexDirection: 'column',
    padding: '8px 8px',
    gap: 2,
    WebkitAppRegion: 'no-drag',
  },
  navItem: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    padding: '7px 10px',
    fontSize: 13,
    color: 'rgba(255,255,255,0.55)',
    background: 'none',
    border: 'none',
    borderRadius: 6,
    cursor: 'pointer',
    textAlign: 'left',
    transition: 'all 0.15s',
    fontWeight: 400,
  },
  navItemActive: {
    background: 'rgba(255,255,255,0.1)',
    color: '#fff',
    fontWeight: 500,
  },
  navIcon: {
    fontSize: 15,
    width: 20,
    textAlign: 'center',
    opacity: 0.8,
  },
  main: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
  },
  titleBar: {
    height: 38,
    WebkitAppRegion: 'drag',
    flexShrink: 0,
  },
  content: {
    flex: 1,
    padding: '0 28px 28px',
    overflowY: 'auto',
  },
}
