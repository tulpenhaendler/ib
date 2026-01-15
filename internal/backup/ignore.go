package backup

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// IgnoreMatcher matches paths against ignore patterns
type IgnoreMatcher struct {
	patterns []ignorePattern
}

type ignorePattern struct {
	pattern  string
	negation bool
	dirOnly  bool
}

// NewIgnoreMatcher creates a new ignore matcher
func NewIgnoreMatcher() *IgnoreMatcher {
	return &IgnoreMatcher{
		patterns: make([]ignorePattern, 0),
	}
}

// LoadFile loads ignore patterns from a file
func (m *IgnoreMatcher) LoadFile(path string) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		pattern := ignorePattern{pattern: line}

		// Check for negation
		if strings.HasPrefix(line, "!") {
			pattern.negation = true
			pattern.pattern = line[1:]
		}

		// Check for directory-only match
		if strings.HasSuffix(pattern.pattern, "/") {
			pattern.dirOnly = true
			pattern.pattern = strings.TrimSuffix(pattern.pattern, "/")
		}

		m.patterns = append(m.patterns, pattern)
	}

	return scanner.Err()
}

// Match checks if a path should be ignored
func (m *IgnoreMatcher) Match(path string, isDir bool) bool {
	// Normalize path separators
	path = filepath.ToSlash(path)

	ignored := false
	for _, p := range m.patterns {
		if p.dirOnly && !isDir {
			continue
		}

		if matchPattern(p.pattern, path) {
			ignored = !p.negation
		}
	}

	return ignored
}

// matchPattern matches a path against a gitignore-style pattern
func matchPattern(pattern, path string) bool {
	// Handle patterns with leading slash (relative to root)
	if strings.HasPrefix(pattern, "/") {
		pattern = pattern[1:]
		return matchGlob(pattern, path)
	}

	// Handle patterns with slash (match in any directory)
	if strings.Contains(pattern, "/") {
		return matchGlob(pattern, path) || strings.HasSuffix(path, "/"+pattern)
	}

	// Simple pattern - match filename anywhere
	base := filepath.Base(path)
	return matchGlob(pattern, base) || matchGlob(pattern, path)
}

// matchGlob matches a path against a glob pattern
func matchGlob(pattern, path string) bool {
	// Handle ** (match any path)
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := parts[0]
			suffix := parts[1]

			if prefix != "" && !strings.HasPrefix(path, prefix) {
				return false
			}
			if suffix != "" {
				suffix = strings.TrimPrefix(suffix, "/")
				return strings.HasSuffix(path, suffix) || containsMatch(path, suffix)
			}
			return true
		}
	}

	matched, _ := filepath.Match(pattern, path)
	if matched {
		return true
	}

	// Also try matching against the full path
	matched, _ = filepath.Match(pattern, filepath.Base(path))
	return matched
}

func containsMatch(path, pattern string) bool {
	parts := strings.Split(path, "/")
	for i := range parts {
		subpath := strings.Join(parts[i:], "/")
		if matched, _ := filepath.Match(pattern, subpath); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, parts[i]); matched {
			return true
		}
	}
	return false
}

// Clone creates a copy of the matcher
func (m *IgnoreMatcher) Clone() *IgnoreMatcher {
	clone := &IgnoreMatcher{
		patterns: make([]ignorePattern, len(m.patterns)),
	}
	copy(clone.patterns, m.patterns)
	return clone
}
