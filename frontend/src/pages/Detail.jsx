import { useState, useEffect } from 'preact/hooks'
import { Link } from 'preact-router/match'
import { fetchManifest, getDownloadUrl } from '../api'
import { formatSize, formatRelativeDate } from '../utils'

export function Detail({ id }) {
  const [manifest, setManifest] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [activeTab, setActiveTab] = useState('http')

  useEffect(() => {
    setLoading(true)
    fetchManifest(id)
      .then((data) => {
        setManifest(data)
        setLoading(false)
      })
      .catch((err) => {
        setError(err.message)
        setLoading(false)
      })
  }, [id])

  if (loading) {
    return (
      <div class="card">
        <div class="empty-state">Loading...</div>
      </div>
    )
  }

  if (error) {
    return (
      <>
        <Link href="/" class="back-link">
          ← Back to list
        </Link>
        <div class="card">
          <div class="empty-state">Failed to load backup: {error}</div>
        </div>
      </>
    )
  }

  const manifestId = manifest.ID || manifest.id
  const date = new Date(manifest.CreatedAt || manifest.created_at)
  const tags = manifest.Tags || manifest.tags || {}
  const entries = manifest.Entries || manifest.entries || []
  const displayName = tags.name || manifestId
  const displayTags = Object.entries(tags).filter(([k]) => k !== 'name')

  const files = entries.filter((e) => (e.Type || e.type) === 'file')
  const dirs = entries.filter((e) => (e.Type || e.type) === 'dir')
  const totalSize = files.reduce((sum, e) => sum + (e.Size || e.size || 0), 0)

  const origin = typeof window !== 'undefined' ? window.location.origin : ''

  return (
    <>
      <Link href="/" class="back-link">
        ← Back to list
      </Link>
      <div class="card">
        <div class="detail-header">
          <h2>{displayName}</h2>
          <div>
            {displayTags.map(([k, v]) => (
              <span key={k} class="tag">
                {k}={v}
              </span>
            ))}
          </div>
        </div>

        <div class="info-grid">
          <div class="info-item">
            <label>Created</label>
            <span>{formatRelativeDate(date)}</span>
          </div>
          <div class="info-item">
            <label>Manifest ID</label>
            <span style={{ fontFamily: 'monospace', fontSize: '0.85rem' }}>{manifestId}</span>
          </div>
          <div class="info-item">
            <label>Files</label>
            <span>{files.length.toLocaleString()}</span>
          </div>
          <div class="info-item">
            <label>Directories</label>
            <span>{dirs.length.toLocaleString()}</span>
          </div>
          <div class="info-item">
            <label>Total Size</label>
            <span>{formatSize(totalSize)}</span>
          </div>
        </div>

        <div class="download-section">
          <h2>Download</h2>
          <div class="tabs">
            <button
              class={`tab ${activeTab === 'http' ? 'active' : ''}`}
              onClick={() => setActiveTab('http')}
            >
              HTTP Download
            </button>
            <button
              class={`tab ${activeTab === 'cli' ? 'active' : ''}`}
              onClick={() => setActiveTab('cli')}
            >
              CLI
            </button>
          </div>

          <div class={`tab-content ${activeTab === 'http' ? 'active' : ''}`}>
            <p style={{ color: '#64748b', fontSize: '0.9rem', marginBottom: '0.75rem' }}>
              Download the entire backup as an archive:
            </p>
            <div class="download-buttons">
              <a href={getDownloadUrl(manifestId, 'tar.gz')} class="btn btn-primary">
                <DownloadIcon />
                Download .tar.gz
              </a>
              <a href={getDownloadUrl(manifestId, 'zip')} class="btn btn-secondary">
                <DownloadIcon />
                Download .zip
              </a>
            </div>
          </div>

          <div class={`tab-content ${activeTab === 'cli' ? 'active' : ''}`}>
            <p style={{ color: '#64748b', fontSize: '0.9rem' }}>
              Use the ib CLI for faster downloads with resume support:
            </p>
            <pre>
              <code>
{`# Connect to server (one time)
ib login ${origin}

# Restore this backup
ib backup restore --id ${manifestId} ./restore-dir`}
              </code>
            </pre>
            <div style={{ marginTop: '1rem' }}>
              <a href="/cli/linux/amd64" class="btn btn-secondary">
                Download CLI (Linux)
              </a>{' '}
              <a href="/cli/darwin/arm64" class="btn btn-secondary">
                Download CLI (macOS)
              </a>
            </div>
          </div>
        </div>
      </div>
    </>
  )
}

function DownloadIcon() {
  return (
    <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16">
      <path d="M.5 9.9a.5.5 0 0 1 .5.5v2.5a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-2.5a.5.5 0 0 1 1 0v2.5a2 2 0 0 1-2 2H2a2 2 0 0 1-2-2v-2.5a.5.5 0 0 1 .5-.5z" />
      <path d="M7.646 11.854a.5.5 0 0 0 .708 0l3-3a.5.5 0 0 0-.708-.708L8.5 10.293V1.5a.5.5 0 0 0-1 0v8.793L5.354 8.146a.5.5 0 1 0-.708.708l3 3z" />
    </svg>
  )
}
