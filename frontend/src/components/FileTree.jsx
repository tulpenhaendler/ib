import { useState, useMemo } from 'preact/hooks'
import { formatSize } from '../utils'
import { getFileDownloadUrl, getFolderDownloadUrl } from '../api'

// Build tree structure from flat entries array
function buildTree(entries) {
  const root = { name: '', children: {}, type: 'dir', path: '' }

  for (const entry of entries) {
    const path = entry.Path || entry.path
    const type = entry.Type || entry.type
    const parts = path.split('/')
    let current = root

    for (let i = 0; i < parts.length; i++) {
      const part = parts[i]
      if (!part) continue

      if (!current.children[part]) {
        current.children[part] = {
          name: part,
          children: {},
          type: i === parts.length - 1 ? type : 'dir',
          path: parts.slice(0, i + 1).join('/'),
        }
      }

      if (i === parts.length - 1) {
        // Attach full entry data to leaf node
        current.children[part].entry = entry
        current.children[part].type = type
      }

      current = current.children[part]
    }
  }

  return root
}

// Sort children: folders first, then alphabetically
function sortChildren(children) {
  return Object.values(children).sort((a, b) => {
    if (a.type === 'dir' && b.type !== 'dir') return -1
    if (a.type !== 'dir' && b.type === 'dir') return 1
    return a.name.localeCompare(b.name)
  })
}

function TreeNode({ node, manifestId, depth = 0 }) {
  const [expanded, setExpanded] = useState(depth < 2)
  const hasChildren = Object.keys(node.children).length > 0
  const isDir = node.type === 'dir'
  const entry = node.entry
  const size = entry?.Size || entry?.size || 0

  return (
    <div class="tree-node">
      <div
        class={`tree-item ${isDir ? 'tree-folder' : 'tree-file'}`}
        style={{ paddingLeft: `${depth * 12 + 4}px` }}
        onClick={() => isDir && hasChildren && setExpanded(!expanded)}
      >
        <span class="tree-icon">
          {isDir ? (hasChildren ? (expanded ? <ChevronDown /> : <ChevronRight />) : <FolderIcon />) : <FileIcon />}
        </span>
        <span class="tree-name">{node.name}</span>
        {!isDir && size > 0 && <span class="tree-size">{formatSize(size)}</span>}
        <span class="tree-actions">
          {isDir && node.path ? (
            <>
              <a href={getFolderDownloadUrl(manifestId, node.path, 'tar.gz')} download class="tree-btn" title="Download .tar.gz" onClick={(e) => e.stopPropagation()}>.tar.gz</a>
              <a href={getFolderDownloadUrl(manifestId, node.path, 'zip')} download class="tree-btn" title="Download .zip" onClick={(e) => e.stopPropagation()}>.zip</a>
            </>
          ) : !isDir && node.path ? (
            <>
              <a href={getFileDownloadUrl(manifestId, node.path)} download class="tree-btn" title="Download raw" onClick={(e) => e.stopPropagation()}>raw</a>
            </>
          ) : null}
        </span>
      </div>
      {isDir && expanded && hasChildren && (
        <div class="tree-children">
          {sortChildren(node.children).map((child) => (
            <TreeNode key={child.path} node={child} manifestId={manifestId} depth={depth + 1} />
          ))}
        </div>
      )}
    </div>
  )
}

export function FileTree({ entries, manifestId }) {
  const tree = useMemo(() => buildTree(entries), [entries])
  const children = sortChildren(tree.children)

  if (children.length === 0) {
    return <div class="file-tree-empty">No files in this backup</div>
  }

  return (
    <div class="file-tree">
      <div class="file-tree-header">
        <h3>Files</h3>
        <span class="file-tree-count">{entries.length} items</span>
      </div>
      <div class="file-tree-content">
        {children.map((child) => (
          <TreeNode key={child.path} node={child} manifestId={manifestId} depth={0} />
        ))}
      </div>
    </div>
  )
}

// Icons
function ChevronRight() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
      <path d="M6 12l4-4-4-4v8z" />
    </svg>
  )
}

function ChevronDown() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
      <path d="M4 6l4 4 4-4H4z" />
    </svg>
  )
}

function FolderIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
      <path d="M1 3.5A1.5 1.5 0 0 1 2.5 2h2.764c.958 0 1.76.56 2.311 1.184C7.985 3.648 8.48 4 9 4h4.5A1.5 1.5 0 0 1 15 5.5v7a1.5 1.5 0 0 1-1.5 1.5h-11A1.5 1.5 0 0 1 1 12.5v-9z" />
    </svg>
  )
}

function FileIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
      <path d="M4 0a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V4.707A1 1 0 0 0 13.707 4L10 .293A1 1 0 0 0 9.293 0H4zm5.5 1.5v2a1 1 0 0 0 1 1h2l-3-3z" />
    </svg>
  )
}

