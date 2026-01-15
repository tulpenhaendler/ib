import { useState, useEffect } from 'preact/hooks'
import { Link } from 'preact-router/match'
import { marked } from 'marked'
import { fetchManifest, fetchManifests, getDownloadUrl, getFileDownloadUrl } from '../api'
import { formatSize, formatRelativeDate } from '../utils'
import { FileTree } from '../components/FileTree'

export function Detail({ id }) {
  const [manifest, setManifest] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [activeTab, setActiveTab] = useState('http')
  const [notesHtml, setNotesHtml] = useState(null)
  const [relatedBackups, setRelatedBackups] = useState([])

  useEffect(() => {
    setLoading(true)
    setNotesHtml(null)
    setRelatedBackups([])

    fetchManifest(id)
      .then((data) => {
        setManifest(data)
        setLoading(false)

        // Check for notes.md and fetch it
        const entries = data.Entries || data.entries || []
        const notesEntry = entries.find((e) => {
          const path = e.Path || e.path
          const type = e.Type || e.type
          return type === 'file' && (path === 'notes.md' || path === 'NOTES.md' || path === 'Notes.md')
        })

        if (notesEntry) {
          const manifestId = data.ID || data.id
          const notesPath = notesEntry.Path || notesEntry.path
          fetch(getFileDownloadUrl(manifestId, notesPath))
            .then((res) => res.ok ? res.text() : null)
            .then((text) => {
              if (text) {
                setNotesHtml(marked.parse(text))
              }
            })
            .catch(() => {})
        }

        // Fetch related backups with the same name
        const tags = data.Tags || data.tags || {}
        if (tags.name) {
          fetchManifests()
            .then((allManifests) => {
              const related = (allManifests || [])
                .filter((m) => {
                  const mTags = m.Tags || m.tags || {}
                  const mId = m.ID || m.id
                  return mTags.name === tags.name && mId !== id
                })
                .sort((a, b) => {
                  const dateA = new Date(a.CreatedAt || a.created_at)
                  const dateB = new Date(b.CreatedAt || b.created_at)
                  return dateB - dateA
                })
              setRelatedBackups(related)
            })
            .catch(() => {})
        }
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
  const rootCid = manifest.RootCID || manifest.root_cid || null
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
          {rootCid && (
            <div class="info-item" style={{ gridColumn: '1 / -1' }}>
              <label>IPFS CID</label>
              <span class="cid-display">
                <code>{rootCid}</code>
                <button
                  class="copy-btn"
                  onClick={() => navigator.clipboard.writeText(rootCid)}
                  title="Copy CID"
                >
                  <CopyIcon />
                </button>
              </span>
            </div>
          )}
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

        {notesHtml && (
          <div class="notes-section">
            <div class="notes-header">
              <NoteIcon />
              <span>Notes</span>
            </div>
            <div class="notes-content markdown" dangerouslySetInnerHTML={{ __html: notesHtml }} />
          </div>
        )}

        <FileTree entries={entries} manifestId={manifestId} />

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
            {rootCid && (
              <button
                class={`tab ${activeTab === 'ipfs' ? 'active' : ''}`}
                onClick={() => setActiveTab('ipfs')}
              >
                IPFS
              </button>
            )}
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

          {rootCid && (
            <div class={`tab-content ${activeTab === 'ipfs' ? 'active' : ''}`}>
              <p style={{ color: '#64748b', fontSize: '0.9rem' }}>
                Access this backup via IPFS using the content ID:
              </p>
              <pre>
                <code>
{`# Via public gateway
https://ipfs.io/ipfs/${rootCid}

# Via local gateway (if running)
http://localhost:8081/ipfs/${rootCid}

# Using IPFS CLI
ipfs get ${rootCid}

# Using kubo
ipfs cat ${rootCid}/<path/to/file>`}
                </code>
              </pre>
              <div style={{ marginTop: '1rem' }}>
                <a
                  href={`https://ipfs.io/ipfs/${rootCid}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="btn btn-secondary"
                >
                  <IpfsIcon />
                  Open on ipfs.io
                </a>
              </div>
            </div>
          )}
        </div>

        {relatedBackups.length > 0 && (
          <div class="versions-section">
            <h3>Previous Versions</h3>
            <div class="versions-list">
              {relatedBackups.map((m) => {
                const mId = m.ID || m.id
                const mDate = new Date(m.CreatedAt || m.created_at)
                return (
                  <Link key={mId} href={`/backup/${mId}`} class="version-item">
                    <span class="version-date">{formatRelativeDate(mDate)}</span>
                    <span class="version-id">{mId}</span>
                  </Link>
                )
              })}
            </div>
          </div>
        )}
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

function NoteIcon() {
  return (
    <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16">
      <path d="M5 4a.5.5 0 0 0 0 1h6a.5.5 0 0 0 0-1H5zm-.5 2.5A.5.5 0 0 1 5 6h6a.5.5 0 0 1 0 1H5a.5.5 0 0 1-.5-.5zM5 8a.5.5 0 0 0 0 1h6a.5.5 0 0 0 0-1H5zm0 2a.5.5 0 0 0 0 1h3a.5.5 0 0 0 0-1H5z"/>
      <path d="M2 2a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V2zm10-1H4a1 1 0 0 0-1 1v12a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1V2a1 1 0 0 0-1-1z"/>
    </svg>
  )
}

function CopyIcon() {
  return (
    <svg width="14" height="14" fill="currentColor" viewBox="0 0 16 16">
      <path d="M4 1.5H3a2 2 0 0 0-2 2V14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V3.5a2 2 0 0 0-2-2h-1v1h1a1 1 0 0 1 1 1V14a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V3.5a1 1 0 0 1 1-1h1v-1z"/>
      <path d="M9.5 1a.5.5 0 0 1 .5.5v1a.5.5 0 0 1-.5.5h-3a.5.5 0 0 1-.5-.5v-1a.5.5 0 0 1 .5-.5h3zm-3-1A1.5 1.5 0 0 0 5 1.5v1A1.5 1.5 0 0 0 6.5 4h3A1.5 1.5 0 0 0 11 2.5v-1A1.5 1.5 0 0 0 9.5 0h-3z"/>
    </svg>
  )
}

function IpfsIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" style={{ marginRight: '0.5rem' }}>
      <path d="M12 0L1.608 6v12L12 24l10.392-6V6L12 0zm-1.073 1.445h.001a1.8 1.8 0 0 0 2.138 0l7.534 4.35a1.794 1.794 0 0 0 0 .403l-7.535 4.35a1.8 1.8 0 0 0-2.137 0l-7.536-4.35a1.795 1.795 0 0 0 0-.402l7.535-4.35zm-9.194 5.7a1.794 1.794 0 0 0 1.07.349l.001 8.7a1.8 1.8 0 0 0 1.07 1.852l-.002-8.701a1.794 1.794 0 0 0-1.068-1.852l7.535-4.35a1.8 1.8 0 0 0 1.069-.349L3.87 7.145a1.794 1.794 0 0 0-1.068.349l-1.069-.349zm18.534 0l-1.069.349a1.794 1.794 0 0 0-1.068-.349l-7.535 4.35a1.8 1.8 0 0 0 1.069.349l-.002 8.701a1.8 1.8 0 0 0 1.07-1.852l.001-8.7a1.794 1.794 0 0 0 1.07-.349l1.069.349a1.794 1.794 0 0 0-1.069-.349l7.535-4.35z"/>
    </svg>
  )
}
