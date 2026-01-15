import { useState, useEffect, useMemo } from 'preact/hooks'
import { Link } from 'preact-router/match'
import { fetchManifests } from '../api'
import { formatRelativeDate } from '../utils'

export function List() {
  const [manifests, setManifests] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [filters, setFilters] = useState({})

  useEffect(() => {
    fetchManifests()
      .then((data) => {
        setManifests(data || [])
        setLoading(false)
      })
      .catch((err) => {
        setError(err.message)
        setLoading(false)
      })
  }, [])

  // Extract unique tag keys and values (excluding 'name')
  const tagValues = useMemo(() => {
    const tags = {}
    manifests.forEach((m) => {
      const mTags = m.Tags || m.tags || {}
      Object.entries(mTags).forEach(([k, v]) => {
        if (k === 'name') return
        if (!tags[k]) tags[k] = new Set()
        tags[k].add(v)
      })
    })
    return tags
  }, [manifests])

  // Filter and group manifests by name (show only latest per name)
  const filtered = useMemo(() => {
    // First filter by tags
    const filteredList = manifests
      .filter((m) => {
        const tags = m.Tags || m.tags || {}
        // Only include manifests with a name tag
        if (!tags.name) return false
        return Object.entries(filters).every(([k, v]) => !v || tags[k] === v)
      })
      .sort((a, b) => {
        const dateA = new Date(a.CreatedAt || a.created_at)
        const dateB = new Date(b.CreatedAt || b.created_at)
        return dateB - dateA
      })

    // Group by name, keep only the latest
    const byName = new Map()
    filteredList.forEach((m) => {
      const tags = m.Tags || m.tags || {}
      const name = tags.name
      if (!byName.has(name)) {
        // Count total backups with this name
        const count = filteredList.filter((x) => (x.Tags || x.tags || {}).name === name).length
        byName.set(name, { manifest: m, count })
      }
    })

    return Array.from(byName.values())
  }, [manifests, filters])

  const setFilter = (key, value) => {
    setFilters((prev) => ({ ...prev, [key]: value || undefined }))
  }

  if (loading) {
    return (
      <div class="card">
        <div class="empty-state">Loading...</div>
      </div>
    )
  }

  if (error) {
    return (
      <div class="card">
        <div class="empty-state">Failed to load backups: {error}</div>
      </div>
    )
  }

  const tagKeys = Object.keys(tagValues).sort()

  return (
    <div class="card">
      <h2>Backups</h2>

      {tagKeys.length > 0 && (
        <div class="filters">
          {tagKeys.map((key) => (
            <div class="filter-group" key={key}>
              <label>{key}</label>
              <select
                value={filters[key] || ''}
                onChange={(e) => setFilter(key, e.target.value)}
              >
                <option value="">All</option>
                {[...tagValues[key]].sort().map((v) => (
                  <option key={v} value={v}>
                    {v}
                  </option>
                ))}
              </select>
            </div>
          ))}
        </div>
      )}

      {filtered.length === 0 ? (
        <div class="empty-state">No backups found</div>
      ) : (
        filtered.map(({ manifest: m, count }) => {
          const id = m.ID || m.id
          const date = new Date(m.CreatedAt || m.created_at)
          const tags = m.Tags || m.tags || {}
          const displayName = tags.name
          const displayTags = Object.entries(tags).filter(([k]) => k !== 'name')

          return (
            <Link key={id} href={`/backup/${id}`} class="backup-item">
              <div class="backup-item-header">
                <h3>{displayName}</h3>
                {count > 1 && <span class="backup-count">{count} versions</span>}
              </div>
              <div class="backup-meta">{formatRelativeDate(date)}</div>
              <div>
                {displayTags.map(([k, v]) => (
                  <span key={k} class="tag">
                    {k}={v}
                  </span>
                ))}
              </div>
            </Link>
          )
        })
      )}

      <div class="cli-section">
        <h3>CLI Client</h3>
        <p>Download the CLI to create and restore backups:</p>
        <div class="cli-downloads">
          <a href="/cli/linux/amd64" class="cli-link">
            <DownloadIcon /> Linux (x64)
          </a>
          <a href="/cli/linux/arm64" class="cli-link">
            <DownloadIcon /> Linux (ARM64)
          </a>
          <a href="/cli/darwin/arm64" class="cli-link">
            <DownloadIcon /> macOS (Apple Silicon)
          </a>
          <a href="/cli/darwin/amd64" class="cli-link">
            <DownloadIcon /> macOS (Intel)
          </a>
          <a href="/cli/windows/amd64" class="cli-link">
            <DownloadIcon /> Windows (x64)
          </a>
        </div>
      </div>
    </div>
  )
}

function DownloadIcon() {
  return (
    <svg width="14" height="14" fill="currentColor" viewBox="0 0 16 16">
      <path d="M.5 9.9a.5.5 0 0 1 .5.5v2.5a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-2.5a.5.5 0 0 1 1 0v2.5a2 2 0 0 1-2 2H2a2 2 0 0 1-2-2v-2.5a.5.5 0 0 1 .5-.5z" />
      <path d="M7.646 11.854a.5.5 0 0 0 .708 0l3-3a.5.5 0 0 0-.708-.708L8.5 10.293V1.5a.5.5 0 0 0-1 0v8.793L5.354 8.146a.5.5 0 1 0-.708.708l3 3z" />
    </svg>
  )
}
