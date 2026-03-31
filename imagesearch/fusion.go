package imagesearch

import "sort"

const rrfK = 60.0 // Reciprocal Rank Fusion constant (matches Rust + websearch)

// fuseWRR merges result sets using Weighted Reciprocal Rank Fusion.
// Deduplicates by URL (first occurrence metadata kept). Sorts by score descending.
func fuseWRR(resultSets [][]ImageResult) []ImageResult {
	if len(resultSets) == 0 {
		return nil
	}

	type scored struct {
		result ImageResult
		score  float64
	}

	byURL := make(map[string]*scored)
	var order []string

	for _, set := range resultSets {
		for rank, r := range set {
			if r.URL == "" {
				continue
			}
			rrf := 1.0 / (rrfK + float64(rank))
			if e, ok := byURL[r.URL]; ok {
				e.score += rrf
			} else {
				order = append(order, r.URL)
				byURL[r.URL] = &scored{result: r, score: rrf}
			}
		}
	}

	merged := make([]ImageResult, 0, len(order))
	for _, u := range order {
		merged = append(merged, byURL[u].result)
	}

	sort.SliceStable(merged, func(i, j int) bool {
		return byURL[merged[i].URL].score > byURL[merged[j].URL].score
	})

	return merged
}
