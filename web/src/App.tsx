import { Routes, Route, NavLink } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import TraceView from './pages/TraceView'
import ErrorBoundary from './ErrorBoundary'

export default function App() {
  return (
    <ErrorBoundary>
      <nav className="nav">
        <NavLink to="/" className="nav-brand">
          <span className="nav-brand-icon">&#9670;</span>
          AgentTrace
        </NavLink>
        <div className="nav-links">
          <NavLink to="/" end className={({ isActive }) => `nav-link${isActive ? ' active' : ''}`}>
            Traces
          </NavLink>
        </div>
        <div className="nav-spacer" />
        <div className="nav-badge">
          <span className="nav-badge-dot" />
          local
        </div>
      </nav>
      <main className="page">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/trace/:id" element={<TraceView />} />
        </Routes>
      </main>
    </ErrorBoundary>
  )
}
