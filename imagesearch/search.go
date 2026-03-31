package imagesearch

import (
	"context"
	"log/slog"
	"sync"
)

// MultiSearch runs multiple ImageEngines in parallel and fuses results via WRR.
type MultiSearch struct {
	Engines      []ImageEngine
	Doer         BrowserDoer // primary (e.g. proxy-based stealth client)
	FallbackDoer BrowserDoer // optional: used when primary fails (e.g. direct HTTP)
}

// Search queries all engines in parallel, fuses via WRR, and truncates to max.
// Engine failures trigger FallbackDoer retry if configured; otherwise skipped.
func (ms *MultiSearch) Search(ctx context.Context, query string, max int) []ImageResult {
	if len(ms.Engines) == 0 {
		return nil
	}

	var mu sync.Mutex
	var allSets [][]ImageResult
	var wg sync.WaitGroup

	for _, eng := range ms.Engines {
		wg.Add(1)
		go func(e ImageEngine) {
			defer wg.Done()
			results, err := e.Search(ctx, ms.Doer, query, max)
			if err != nil && ms.FallbackDoer != nil {
				slog.Warn("imagesearch: primary failed, trying fallback",
					"engine", e.Name(), "error", err)
				results, err = e.Search(ctx, ms.FallbackDoer, query, max)
			}
			if err != nil {
				slog.Warn("imagesearch: engine failed",
					"engine", e.Name(), "error", err)
				return
			}
			if len(results) == 0 && ms.FallbackDoer != nil {
				slog.Debug("imagesearch: primary returned 0, trying fallback",
					"engine", e.Name())
				results, _ = e.Search(ctx, ms.FallbackDoer, query, max)
			}
			if len(results) > 0 {
				mu.Lock()
				allSets = append(allSets, results)
				mu.Unlock()
			}
		}(eng)
	}
	wg.Wait()

	fused := fuseWRR(allSets)
	if len(fused) > max {
		fused = fused[:max]
	}
	return fused
}
