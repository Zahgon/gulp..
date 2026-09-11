package cli

import "sort"

// maxSuggestionDistance is the edit distance below which a registered task is
// considered a plausible typo of what the user typed. Three edits catches the
// realistic mistakes — a dropped letter, a transposition, a wrong suffix —
// without suggesting "clean" for "watch".
const maxSuggestionDistance = 3

// SimilarTasks returns the registered names closest to name, for the
// "did you mean?" hint gulp prints when a task is not found.
func SimilarTasks(name string, candidates []string) []string {
	type scored struct {
		name     string
		distance int
	}
	var matches []scored
	for _, candidate := range candidates {
		d := levenshtein(name, candidate)
		if d <= maxSuggestionDistance {
			matches = append(matches, scored{candidate, d})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].distance != matches[j].distance {
			return matches[i].distance < matches[j].distance
		}
		return matches[i].name < matches[j].name
	})
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.name)
	}
	return out
}

// levenshtein computes the edit distance between two strings using a single
// rolling row, which is all the full matrix is needed for when only the final
// distance matters.
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}

	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}
