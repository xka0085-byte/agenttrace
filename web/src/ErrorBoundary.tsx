import React, { Component, type ReactNode } from 'react'

interface Props { children: ReactNode; fallback?: ReactNode }
interface State { hasError: boolean; error: Error | null }

export default class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error('[ErrorBoundary]', error, info)
  }

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) return this.props.fallback
      return (
        <div className="empty-state">
          <div className="empty-state-icon" style={{ background: 'var(--crimson-bg)', borderColor: 'var(--crimson)' }}>
            <span style={{ color: 'var(--crimson)' }}>&#9888;</span>
          </div>
          <div className="empty-state-title">Something went wrong</div>
          <div className="empty-state-desc">
            {this.state.error?.message || 'An unexpected error occurred.'}
          </div>
          <button
            className="btn btn-primary"
            onClick={() => this.setState({ hasError: false, error: null })}
          >
            Try Again
          </button>
        </div>
      )
    }
    return this.props.children
  }
}
