package aql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"ecosystem-server/common"
)

type aqlResponse struct {
	Results []aqlResult `json:"results"`
}

type aqlResult struct {
	Repo       string `json:"repo"`
	Path       string `json:"path"`
	Name       string `json:"name"`
	ActualSHA1 string `json:"actual_sha1"`
	SHA256     string `json:"sha256"`
	ActualMD5  string `json:"actual_md5"`
}

type depWithParts struct {
	dep   common.Dependency
	parts common.TarballParts
}

func FetchChecksums(deps []common.Dependency, cfg common.Config) ([]common.DependencyResult, common.FetchStats) {
	overallStart := time.Now()

	var parsed []depWithParts
	for _, d := range deps {
		parts, err := common.ParseTarballURL(d.ResolvedURL)
		if err != nil {
			parts = common.BuildTarballPartsFromName(d.Name, d.Version)
			if d.Repo != "" {
				parts.Repo = d.Repo
			}
		}
		parsed = append(parsed, depWithParts{dep: d, parts: parts})
	}

	// Group by repo — each repo needs a separate AQL query
	repoGroups := make(map[string][]depWithParts)
	for _, dp := range parsed {
		cacheRepo := toCacheRepo(dp.parts.Repo)
		repoGroups[cacheRepo] = append(repoGroups[cacheRepo], dp)
	}

	var (
		mu           sync.Mutex
		resultMap    = make(map[string]aqlResult)
		apiCalls     int
		batchTimings []common.PackageTiming
	)

	client := &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}
	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup

	for repo, group := range repoGroups {
		batches := batchDeps(group, cfg.AQLBatchSize)

		for batchIdx, batch := range batches {
			wg.Add(1)
			sem <- struct{}{}

			go func(repo string, idx int, batch []depWithParts) {
				defer wg.Done()
				defer func() { <-sem }()

				batchStart := time.Now()
				results, err := executeBatchAQL(client, cfg, repo, batch)
				batchDuration := time.Since(batchStart)

				mu.Lock()
				apiCalls++
				if err == nil {
					perPkg := batchDuration / time.Duration(len(batch))
					for _, r := range results {
						key := r.Path + "/" + r.Name
						resultMap[key] = r
					}
					for _, d := range batch {
						batchTimings = append(batchTimings, common.PackageTiming{
							Name:     d.dep.Name,
							Version:  d.dep.Version,
							Duration: perPkg,
						})
					}
				} else {
					fmt.Printf("  [AQL] repo=%s batch %d failed: %v\n", repo, idx, err)
					for _, d := range batch {
						batchTimings = append(batchTimings, common.PackageTiming{
							Name:     d.dep.Name,
							Version:  d.dep.Version,
							Duration: batchDuration / time.Duration(len(batch)),
						})
					}
				}
				mu.Unlock()
			}(repo, batchIdx, batch)
		}
	}

	wg.Wait()
	totalTime := time.Since(overallStart)

	var results []common.DependencyResult
	success, fail := 0, 0

	for _, dp := range parsed {
		key := dp.parts.DirPath + "/" + dp.parts.FileName
		r := common.DependencyResult{Dependency: dp.dep}

		if aqlRes, ok := resultMap[key]; ok {
			r.Checksums = common.Checksums{
				SHA1:   aqlRes.ActualSHA1,
				SHA256: aqlRes.SHA256,
				MD5:    aqlRes.ActualMD5,
			}
			success++
		} else {
			fail++
		}
		results = append(results, r)
	}

	stats := common.ComputeStats("AQL (Bulk)", batchTimings, totalTime, apiCalls, success, fail)
	return results, stats
}

func executeBatchAQL(client *http.Client, cfg common.Config, repo string, batch []depWithParts) ([]aqlResult, error) {
	query := buildAQLQuery(repo, batch)

	url := strings.TrimRight(cfg.ArtifactoryURL, "/") + "/api/search/aql"
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(query))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", "Bearer "+cfg.ArtifactoryToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing AQL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AQL returned %d: %s", resp.StatusCode, string(body))
	}

	var aqlResp aqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&aqlResp); err != nil {
		return nil, fmt.Errorf("decoding AQL response: %w", err)
	}

	return aqlResp.Results, nil
}

func buildAQLQuery(repo string, batch []depWithParts) string {
	var clauses []string
	for _, dp := range batch {
		clause := fmt.Sprintf(
			`{"$and":[{"path":"%s"},{"name":"%s"}]}`,
			dp.parts.DirPath, dp.parts.FileName,
		)
		clauses = append(clauses, clause)
	}

	return fmt.Sprintf(
		`items.find({"repo":"%s","$or":[%s]}).include("repo","path","name","actual_sha1","sha256","actual_md5")`,
		repo, strings.Join(clauses, ","),
	)
}

// toCacheRepo converts a virtual repo name to its cache repo name.
// Artifactory stores cached artifacts in <repo>-cache.
func toCacheRepo(repo string) string {
	if repo == "" {
		return ""
	}
	if strings.HasSuffix(repo, "-cache") {
		return repo
	}
	return repo + "-cache"
}

func batchDeps(deps []depWithParts, batchSize int) [][]depWithParts {
	if batchSize <= 0 {
		batchSize = 30
	}
	var batches [][]depWithParts
	for i := 0; i < len(deps); i += batchSize {
		end := i + batchSize
		if end > len(deps) {
			end = len(deps)
		}
		batches = append(batches, deps[i:end])
	}
	return batches
}
