const API_BASE = '/api'

export async function fetchConfig() {
  const res = await fetch(`${API_BASE}/config`)
  if (!res.ok) return { title: 'ib Backup' }
  return res.json()
}

export async function fetchManifests() {
  const res = await fetch(`${API_BASE}/manifests`)
  if (!res.ok) throw new Error('Failed to fetch manifests')
  return res.json()
}

export async function fetchManifest(id) {
  const res = await fetch(`${API_BASE}/manifests/${id}`)
  if (!res.ok) throw new Error('Failed to fetch manifest')
  return res.json()
}

export function getDownloadUrl(id, format) {
  return `${API_BASE}/download/${id}.${format}`
}

export function getFileDownloadUrl(manifestId, path) {
  return `${API_BASE}/download/${manifestId}/file/${path}`
}

export function getFolderDownloadUrl(manifestId, path, format = 'tar.gz') {
  return `${API_BASE}/download/${manifestId}/folder/${path}.${format}`
}
