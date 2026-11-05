import { useEffect } from 'react'
import { useLocation } from 'react-router'
import SessionList from './analysis/list'
import RealOverview from './real-overview'
import './dashboard.css'

/** A single page for real workspace progress and persisted session analysis. */
export default function Page() {
  const { hash, pathname } = useLocation()
  useEffect(() => {
    const target = hash || (pathname === '/dashboard/sessions' ? '#session-analysis' : '')
    if (target) document.getElementById(target.slice(1))?.scrollIntoView?.()
  }, [hash, pathname])
  return <main className="dashboard-feature db-page">
    <div className="db-content">
      <header className="db-page-header">
        <div>
          <span className="db-eyebrow">工作空间 / 运行洞察</span>
          <h1>Dashboard</h1>
          <p>从团队协作进度，到每一次 Agent 会话的执行细节。</p>
        </div>
        <nav className="db-section-links" aria-label="Dashboard 分区">
          <a href="#team-overview">团队概览</a>
          <a href="#session-analysis">会话分析 ↓</a>
        </nav>
      </header>
      <section id="team-overview" aria-label="团队概览" className="db-team-section">
        <RealOverview />
      </section>
      <SessionList />
    </div>
  </main>
}
