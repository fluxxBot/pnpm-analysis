package common

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func ComputeStats(approach string, timings []PackageTiming, totalTime time.Duration, apiCalls, success, fail int) FetchStats {
	stats := FetchStats{
		Approach:       approach,
		TotalDeps:      success + fail,
		SuccessCount:   success,
		FailCount:      fail,
		APICallCount:   apiCalls,
		TotalTime:      totalTime,
		PackageTimings: timings,
	}

	if len(timings) == 0 {
		return stats
	}

	sort.Slice(timings, func(i, j int) bool {
		return timings[i].Duration < timings[j].Duration
	})

	var total time.Duration
	for _, t := range timings {
		total += t.Duration
	}

	stats.AvgPerPackage = total / time.Duration(len(timings))
	stats.MinPerPackage = timings[0].Duration
	stats.MaxPerPackage = timings[len(timings)-1].Duration
	stats.P50PerPackage = timings[percentileIndex(len(timings), 50)].Duration
	stats.P95PerPackage = timings[percentileIndex(len(timings), 95)].Duration
	stats.P99PerPackage = timings[percentileIndex(len(timings), 99)].Duration

	return stats
}

func percentileIndex(n, p int) int {
	idx := (p * n) / 100
	if idx >= n {
		idx = n - 1
	}
	return idx
}

func PrintStats(s FetchStats) {
	fmt.Printf("\n%s\n", strings.Repeat("=", 60))
	fmt.Printf("  %s\n", s.Approach)
	fmt.Printf("%s\n", strings.Repeat("=", 60))
	fmt.Printf("  %-25s %d\n", "Total dependencies:", s.TotalDeps)
	fmt.Printf("  %-25s %d\n", "Success:", s.SuccessCount)
	fmt.Printf("  %-25s %d\n", "Failed:", s.FailCount)
	fmt.Printf("  %-25s %d\n", "API calls made:", s.APICallCount)
	fmt.Printf("  %-25s %s\n", "Total wall time:", s.TotalTime.Round(time.Millisecond))
	fmt.Printf("  %-25s %s\n", "Avg per package:", s.AvgPerPackage.Round(time.Microsecond))
	fmt.Printf("  %-25s %s\n", "Min per package:", s.MinPerPackage.Round(time.Microsecond))
	fmt.Printf("  %-25s %s\n", "Max per package:", s.MaxPerPackage.Round(time.Microsecond))
	fmt.Printf("  %-25s %s\n", "P50 per package:", s.P50PerPackage.Round(time.Microsecond))
	fmt.Printf("  %-25s %s\n", "P95 per package:", s.P95PerPackage.Round(time.Microsecond))
	fmt.Printf("  %-25s %s\n", "P99 per package:", s.P99PerPackage.Round(time.Microsecond))
}

func PrintComparison(aql, storage FetchStats) {
	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
	fmt.Printf("  COMPARISON: %s vs %s\n", aql.Approach, storage.Approach)
	fmt.Printf("%s\n", strings.Repeat("=", 80))

	header := fmt.Sprintf("  %-28s %20s %20s", "Metric", aql.Approach, storage.Approach)
	fmt.Println(header)
	fmt.Println(strings.Repeat("-", 80))

	rows := []struct {
		label    string
		aqlVal   string
		storeVal string
	}{
		{"Total dependencies", itoa(aql.TotalDeps), itoa(storage.TotalDeps)},
		{"Success", itoa(aql.SuccessCount), itoa(storage.SuccessCount)},
		{"Failed", itoa(aql.FailCount), itoa(storage.FailCount)},
		{"API calls", itoa(aql.APICallCount), itoa(storage.APICallCount)},
		{"Total wall time", dur(aql.TotalTime), dur(storage.TotalTime)},
		{"Avg per package", dur(aql.AvgPerPackage), dur(storage.AvgPerPackage)},
		{"Min per package", dur(aql.MinPerPackage), dur(storage.MinPerPackage)},
		{"Max per package", dur(aql.MaxPerPackage), dur(storage.MaxPerPackage)},
		{"P50 per package", dur(aql.P50PerPackage), dur(storage.P50PerPackage)},
		{"P95 per package", dur(aql.P95PerPackage), dur(storage.P95PerPackage)},
		{"P99 per package", dur(aql.P99PerPackage), dur(storage.P99PerPackage)},
	}

	for _, r := range rows {
		fmt.Printf("  %-28s %20s %20s\n", r.label, r.aqlVal, r.storeVal)
	}

	if storage.TotalTime > 0 && aql.TotalTime > 0 {
		speedup := float64(storage.TotalTime) / float64(aql.TotalTime)
		fmt.Printf("\n  AQL is %.1fx faster in total wall time\n", speedup)
	}

	if storage.APICallCount > 0 && aql.APICallCount > 0 {
		reduction := float64(storage.APICallCount-aql.APICallCount) / float64(storage.APICallCount) * 100
		fmt.Printf("  AQL reduces API calls by %.1f%% (%d → %d)\n",
			reduction, storage.APICallCount, aql.APICallCount)
	}
}

func PrintDependencyResults(results []DependencyResult, limit int) {
	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	fmt.Printf("\n  Sample results (first %d of %d):\n", limit, len(results))
	fmt.Printf("  %-40s %-12s %-15s %-10s %-10s\n", "Package", "Version", "Scope", "SHA256", "SHA1")
	fmt.Println(strings.Repeat("-", 100))
	for i := 0; i < limit; i++ {
		r := results[i]
		sha256Short := truncHash(r.Checksums.SHA256, 8)
		sha1Short := truncHash(r.Checksums.SHA1, 8)
		fmt.Printf("  %-40s %-12s %-15s %-10s %-10s\n",
			r.Name, r.Version, r.Scope, sha256Short, sha1Short)
	}
}

func itoa(n int) string  { return fmt.Sprintf("%d", n) }
func dur(d time.Duration) string { return d.Round(time.Microsecond).String() }

func truncHash(h string, n int) string {
	if len(h) <= n {
		return h
	}
	return h[:n] + "..."
}
