// Package workspace confines every file operation to one directory.
//
// A model that can read /etc/passwd or write outside its project is not a
// coding agent, it is a liability. Every path a tool receives goes through
// Resolve first; anything that escapes the root is refused before any I/O.
package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrOutside is returned for any path that leaves the workspace root.
var ErrOutside = errors.New("workspace: path outside root")

// InstructionsFile is read from the workspace root into the system prompt.
const InstructionsFile = "AGENTS.md"

// Workspace is one root directory with a resolved real path.
type Workspace struct {
	root string
}

// Open resolves root (defaulting to the working directory) and checks it is a
// directory. Symlinks are resolved once here so later comparisons are stable.
func Open(root string) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace: %s is not a directory", abs)
	}
	return &Workspace{root: abs}, nil
}

// Root returns the resolved root directory.
func (w *Workspace) Root() string { return w.root }

// Resolve turns a user or model supplied path into an absolute path inside the
// workspace. Absolute paths are accepted only when they are already inside.
func (w *Workspace) Resolve(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return w.root, nil
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(w.root, p)
	}
	p = filepath.Clean(p)

	if !w.inside(p) {
		return "", fmt.Errorf("%w: %s", ErrOutside, p)
	}
	// Follow symlinks when the target exists, otherwise check its parent: a
	// link inside the tree pointing outside is still an escape.
	if real, err := filepath.EvalSymlinks(p); err == nil {
		if !w.inside(real) {
			return "", fmt.Errorf("%w: %s", ErrOutside, real)
		}
		return real, nil
	}
	if parent, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		if !w.inside(parent) {
			return "", fmt.Errorf("%w: %s", ErrOutside, parent)
		}
	}
	return p, nil
}

func (w *Workspace) inside(p string) bool {
	if p == w.root {
		return true
	}
	return strings.HasPrefix(p, w.root+string(os.PathSeparator))
}

// Instructions returns the AGENTS.md text at the root, or "" when absent.
func (w *Workspace) Instructions() string {
	raw, err := os.ReadFile(filepath.Join(w.root, InstructionsFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
