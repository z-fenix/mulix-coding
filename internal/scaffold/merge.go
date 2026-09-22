package scaffold

import "strings"

// This file implements a line-level three-way merge (diff3) used by
// `mulix update`: baseline (what mulix wrote at install time), incoming
// (the new bundled version), and local (the file as it exists on disk,
// possibly edited by the user). Changes on one side merge cleanly;
// overlapping or adjacent changes on both sides produce Git-style
// conflict markers rather than a silent guess.

// match is one aligned line pair between the base and another version:
// base[b] == other[o].
type match struct{ b, o int }

// hunk replaces base[baseStart:baseEnd] with lines (empty lines slice =
// deletion, empty base span = insertion).
type hunk struct {
	baseStart, baseEnd int
	lines              []string
}

// lcsMatches returns the longest common subsequence of base and other as
// ascending (base index, other index) pairs.
func lcsMatches(base, other []string) []match {
	n, m := len(base), len(other)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case base[i] == other[j]:
				dp[i][j] = dp[i+1][j+1] + 1
			case dp[i+1][j] >= dp[i][j+1]:
				dp[i][j] = dp[i+1][j]
			default:
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var out []match
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case base[i] == other[j]:
			out = append(out, match{i, j})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return out
}

// diffHunks computes the hunks that transform base into other.
func diffHunks(base, other []string) []hunk {
	matches := lcsMatches(base, other)
	var hunks []hunk
	bi, oi := 0, 0
	var cur *hunk
	flush := func() {
		if cur != nil {
			hunks = append(hunks, *cur)
			cur = nil
		}
	}
	for mi := 0; ; mi++ {
		mBi, mOi := len(base), len(other)
		if mi < len(matches) {
			mBi, mOi = matches[mi].b, matches[mi].o
		}
		if bi < mBi || oi < mOi {
			if cur == nil {
				cur = &hunk{baseStart: bi}
			}
			cur.baseEnd = mBi
			for ; oi < mOi; oi++ {
				cur.lines = append(cur.lines, other[oi])
			}
			bi = mBi
		}
		if mi == len(matches) {
			break
		}
		flush()
		bi = mBi + 1
		oi = mOi + 1
	}
	flush()
	return hunks
}

// spanText renders one side's version of base[start:end]: hunk-covered
// base ranges are replaced by the hunk's lines, uncovered base lines
// pass through.
func spanText(base []string, hunks []hunk, start, end int) []string {
	var out []string
	bi := start
	for _, h := range hunks {
		for ; bi < h.baseStart; bi++ {
			out = append(out, base[bi])
		}
		out = append(out, h.lines...)
		bi = h.baseEnd
	}
	for ; bi < end; bi++ {
		out = append(out, base[bi])
	}
	return out
}

// merge3 merges an incoming (new bundled) version and a local (on-disk)
// version against their common base. It returns the merged lines and the
// number of conflict regions embedded in them as Git-style markers.
func merge3(base, incoming, local []string) ([]string, int) {
	iHunks := diffHunks(base, incoming)
	lHunks := diffHunks(base, local)

	var result []string
	conflicts := 0
	pos := 0 // base lines before this index are already copied
	ii, li := 0, 0

	for ii < len(iHunks) || li < len(lHunks) {
		start := -1
		if ii < len(iHunks) {
			start = iHunks[ii].baseStart
		}
		if li < len(lHunks) && (start == -1 || lHunks[li].baseStart < start) {
			start = lHunks[li].baseStart
		}
		end := start
		var rI, rL []hunk
		for {
			grew := false
			for ii < len(iHunks) && iHunks[ii].baseStart <= end {
				if iHunks[ii].baseEnd > end {
					end = iHunks[ii].baseEnd
				}
				rI = append(rI, iHunks[ii])
				ii++
				grew = true
			}
			for li < len(lHunks) && lHunks[li].baseStart <= end {
				if lHunks[li].baseEnd > end {
					end = lHunks[li].baseEnd
				}
				rL = append(rL, lHunks[li])
				li++
				grew = true
			}
			if !grew {
				break
			}
		}

		result = append(result, base[pos:start]...)
		pos = end

		switch {
		case len(rL) == 0: // only mulix changed this region
			result = append(result, spanText(base, rI, start, end)...)
		case len(rI) == 0: // only the user changed this region
			result = append(result, spanText(base, rL, start, end)...)
		default:
			iText := spanText(base, rI, start, end)
			lText := spanText(base, rL, start, end)
			if strings.Join(iText, "\n") == strings.Join(lText, "\n") {
				result = append(result, iText...)
			} else {
				conflicts++
				result = append(result, "<<<<<<< incoming (mulix)")
				result = append(result, iText...)
				result = append(result, "=======")
				result = append(result, lText...)
				result = append(result, ">>>>>>> local")
			}
		}
	}

	result = append(result, base[pos:]...)
	return result, conflicts
}
