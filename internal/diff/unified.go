// Package diff renders a unified diff for write previews.
//
// It is deliberately small: a line-level LCS, capped, with one hunk header.
// A preview only needs to be readable and honest; a byte-exact git diff is not
// the goal.
package diff

import (
	"fmt"
	"strings"
)

const maxLines = 4000

// Unified renders before/after as a unified diff. Identical input yields "".
func Unified(path, before, after string) string {
	if before == after {
		return ""
	}
	a := splitLines(before)
	b := splitLines(after)
	if len(a) > maxLines || len(b) > maxLines {
		return fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ preview too large: %d -> %d lines @@\n",
			path, path, len(a), len(b))
	}

	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", path, path)
	fmt.Fprintf(&out, "@@ -1,%d +1,%d @@\n", len(a), len(b))

	for _, op := range diffOps(a, b) {
		switch op.kind {
		case opSame:
			out.WriteString(" " + op.text + "\n")
		case opDel:
			out.WriteString("-" + op.text + "\n")
		case opAdd:
			out.WriteString("+" + op.text + "\n")
		}
	}
	return out.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

type opKind int

const (
	opSame opKind = iota
	opDel
	opAdd
)

type op struct {
	kind opKind
	text string
}

// diffOps is a standard LCS backtrack.
func diffOps(a, b []string) []op {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	out := make([]op, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, op{opSame, a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			out = append(out, op{opDel, a[i]})
			i++
		default:
			out = append(out, op{opAdd, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, op{opDel, a[i]})
	}
	for ; j < m; j++ {
		out = append(out, op{opAdd, b[j]})
	}
	return out
}
