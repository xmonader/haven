package diff

// op is a single line operation in a diff.
type op struct {
	kind byte // ' ' equal, '-' delete, '+' insert
	text string
}

func (o op) sign() string { return string(o.kind) }

// lcsDiff produces a line-level diff of a -> b via a longest-common-subsequence
// dynamic program.
func lcsDiff(a, b []string) []op {
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

	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, op{' ', a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, op{'-', a[i]})
			i++
		default:
			ops = append(ops, op{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, op{'+', b[j]})
	}
	return ops
}

type hunk struct {
	oldStart, oldCount int
	newStart, newCount int
	ops                []op
}

// group collects changed ops into hunks padded with up to context equal lines,
// merging hunks separated by no more than the context gap.
func group(ops []op, context int) []hunk {
	n := len(ops)
	type rng struct{ lo, hi int }
	var ranges []rng
	for i := 0; i < n; i++ {
		if ops[i].kind == ' ' {
			continue
		}
		lo, hi := i-context, i+context
		if lo < 0 {
			lo = 0
		}
		if hi > n-1 {
			hi = n - 1
		}
		if k := len(ranges); k > 0 && lo <= ranges[k-1].hi+1 {
			ranges[k-1].hi = hi
		} else {
			ranges = append(ranges, rng{lo, hi})
		}
	}
	if len(ranges) == 0 {
		return nil
	}

	// Precompute old/new line index at the start of each op.
	oldAt := make([]int, n+1)
	newAt := make([]int, n+1)
	oi, ni := 0, 0
	for i := 0; i < n; i++ {
		oldAt[i], newAt[i] = oi, ni
		switch ops[i].kind {
		case ' ':
			oi++
			ni++
		case '-':
			oi++
		case '+':
			ni++
		}
	}
	oldAt[n], newAt[n] = oi, ni

	var hunks []hunk
	for _, r := range ranges {
		hunks = append(hunks, hunk{
			oldStart: oldAt[r.lo],
			oldCount: oldAt[r.hi+1] - oldAt[r.lo],
			newStart: newAt[r.lo],
			newCount: newAt[r.hi+1] - newAt[r.lo],
			ops:      ops[r.lo : r.hi+1],
		})
	}
	return hunks
}
