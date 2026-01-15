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

  // Filter manifests
  const filtered = useMemo(() => {
    return manifests
      .filter((m) => {
        const tags = m.Tags || m.tags || {}
        return Object.entries(filters).every(([k, v]) => !v || tags[k] === v)
      })
      .sort((a, b) => {
        const dateA = new Date(a.CreatedAt || a.created_at)
        const dateB = new Date(b.CreatedAt || b.created_at)
        return dateB - dateA
      })
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
        filtered.map((m) => {
          const id = m.ID || m.id
          const date = new Date(m.CreatedAt || m.created_at)
          const tags = m.Tags || m.tags || {}
          const displayName = tags.name || id
          const displayTags = Object.entries(tags).filter(([k]) => k !== 'name')

          return (
            <Link key={id} href={`/backup/${id}`} class="backup-item">
              <h3>{displayName}</h3>
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
    </div>
  )
}
