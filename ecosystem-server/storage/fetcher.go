package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ecosystem-server/common"
)

type storageResponse struct {
	Repo      string `json:"repo"`
	Path      string `json:"path"`
	Checksums struct {
		SHA1   string `json:"sha1"`
		SHA256 string `json:"sha256"`
		MD5    string `json:"md5"`
	} `json:"checksums"`
	Size string `json:"size"`
}

func FetchChecksums(deps []common.Dependency, cfg common.Config) ([]common.DependencyResult, common.FetchStats) {
	overallStart := time.Now()

	type workItem struct {
		idx   int
		dep   common.Dependency
		parts common.TarballParts
	}

	items := make([]workItem, 0, len(deps))
	for i, d := range deps {
		parts, err := common.ParseTarballURL(d.ResolvedURL)
		if err != nil {
			parts = common.BuildTarballPartsFromName(d.Name, d.Version)
			if d.Repo != "" {
				parts.Repo = d.Repo
			}
		}
		items = append(items, workItem{idx: i, dep: d, parts: parts})
	}

	results := make([]common.DependencyResult, len(deps))
	timings := make([]common.PackageTiming, len(deps))

	var successCount, failCount atomic.Int64

	client := &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}
	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup

	for _, item := range items {
		wg.Add(1)
		sem <- struct{}{}

		go func(w workItem) {
			defer wg.Done()
			defer func() { <-sem }()

			start := time.Now()
			checksums, err := fetchSingle(client, cfg, w.parts)
			elapsed := time.Since(start)

			r := common.DependencyResult{Dependency: w.dep}
			if err == nil {
				r.Checksums = checksums
				successCount.Add(1)
			} else {
				failCount.Add(1)
			}

			results[w.idx] = r
			timings[w.idx] = common.PackageTiming{
				Name:     w.dep.Name,
				Version:  w.dep.Version,
				Duration: elapsed,
			}
		}(item)
	}

	wg.Wait()
	totalTime := time.Since(overallStart)

	stats := common.ComputeStats(
		"Storage API (Per-Pkg)",
		timings, totalTime,
		len(deps),
		int(successCount.Load()),
		int(failCount.Load()),
	)

	return results, stats
}

func fetchSingle(client *http.Client, cfg common.Config, parts common.TarballParts) (common.Checksums, error) {
	repo := parts.Repo
	if repo == "" {
		return common.Checksums{}, fmt.Errorf("no repo for %s", parts.FullPath)
	}

	// Storage API works with virtual repos (auto-resolves to cache),
	// so use the repo as-is from the resolved URL.
	apiPath := common.StorageAPIPath(repo, parts)
	url := strings.TrimRight(cfg.ArtifactoryURL, "/") + apiPath

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return common.Checksums{}, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.ArtifactoryToken)

	var lastErr error
	for attempt := 0; attempt <= cfg.RetryMax; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * 500 * time.Millisecond
			time.Sleep(backoff)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return common.Checksums{}, fmt.Errorf("HTTP %d for %s: %s", resp.StatusCode, url, string(body))
		}

		var storageResp storageResponse
		if err := json.Unmarshal(body, &storageResp); err != nil {
			return common.Checksums{}, fmt.Errorf("decoding response for %s: %w", url, err)
		}

		return common.Checksums{
			SHA1:   storageResp.Checksums.SHA1,
			SHA256: storageResp.Checksums.SHA256,
			MD5:    storageResp.Checksums.MD5,
		}, nil
	}

	return common.Checksums{}, fmt.Errorf("all retries exhausted for %s: %w", parts.FullPath, lastErr)
}
