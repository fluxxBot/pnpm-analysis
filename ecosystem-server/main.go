package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"ecosystem-server/aql"
	"ecosystem-server/common"
	"ecosystem-server/storage"
)

func main() {
	artURL := flag.String("artifactory-url", "", "Artifactory base URL (e.g. https://mycompany.jfrog.io/artifactory)")
	artToken := flag.String("artifactory-token", "", "Artifactory API token (or set ARTIFACTORY_TOKEN env)")
	projectDir := flag.String("project-dir", "..", "Path to pnpm project directory")
	pnpmJSON := flag.String("pnpm-json", "", "Path to pre-generated pnpm ls JSON (skips running pnpm ls)")
	concurrency := flag.Int("concurrency", 15, "Max concurrent API requests")
	batchSize := flag.Int("batch-size", 30, "AQL batch size ($or clauses per query)")
	retryMax := flag.Int("retry-max", 3, "Max retries per failed request")
	timeout := flag.Int("timeout", 30, "Per-request timeout in seconds")
	mode := flag.String("mode", "both", "Run mode: aql, storage, or both")
	sampleSize := flag.Int("sample", 20, "Number of sample results to print")

	flag.Parse()

	if *artURL == "" {
		*artURL = os.Getenv("ARTIFACTORY_URL")
	}
	if *artToken == "" {
		*artToken = os.Getenv("ARTIFACTORY_TOKEN")
	}
	if *artURL == "" || *artToken == "" {
		fmt.Fprintln(os.Stderr, "Error: --artifactory-url and --artifactory-token (or env vars) are required")
		flag.Usage()
		os.Exit(1)
	}

	wd, _ := os.Getwd()
	fmt.Printf("Working directory: %s\n", wd)

	var deps []common.Dependency
	var err error

	if *pnpmJSON != "" {
		fmt.Printf("Reading pre-generated: %s\n", *pnpmJSON)
		deps, err = common.ParseDependenciesFromFile(*pnpmJSON)
	} else {
		absDir, _ := filepath.Abs(*projectDir)
		fmt.Printf("Running: pnpm ls --depth Infinity --json (in %s)\n", absDir)
		deps, err = common.ParseDependencies(absDir)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing dependencies: %v\n", err)
		os.Exit(1)
	}

	sort.Slice(deps, func(i, j int) bool { return deps[i].Name < deps[j].Name })

	scoped, unscoped := 0, 0
	repos := make(map[string]int)
	for _, d := range deps {
		if len(d.Name) > 0 && d.Name[0] == '@' {
			scoped++
		} else {
			unscoped++
		}
		if d.Repo != "" {
			repos[d.Repo]++
		}
	}
	fmt.Printf("Parsed %d dependencies (scoped: %d, unscoped: %d)\n", len(deps), scoped, unscoped)
	fmt.Printf("Repositories found: ")
	for repo, count := range repos {
		fmt.Printf("%s(%d) ", repo, count)
	}
	fmt.Println()

	cfg := common.Config{
		ArtifactoryURL:   *artURL,
		ArtifactoryToken: *artToken,
		Concurrency:      *concurrency,
		AQLBatchSize:     *batchSize,
		RetryMax:         *retryMax,
		TimeoutSeconds:   *timeout,
	}

	outputDir, _ := filepath.Abs(*projectDir)

	var aqlStats, storageStats common.FetchStats
	var aqlResults, storageResults []common.DependencyResult

	if *mode == "aql" || *mode == "both" {
		fmt.Println("\nRunning AQL bulk fetcher...")
		aqlResults, aqlStats = aql.FetchChecksums(deps, cfg)
		common.PrintStats(aqlStats)
		printResults(aqlResults, *sampleSize)

		outPath := filepath.Join(outputDir, "aql-checksums.json")
		if err := writeJSON(outPath, aqlResults, aqlStats); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing AQL JSON: %v\n", err)
		} else {
			fmt.Printf("\n  AQL results written to: %s\n", outPath)
		}
	}

	if *mode == "storage" || *mode == "both" {
		fmt.Println("\nRunning Storage API per-package fetcher...")
		storageResults, storageStats = storage.FetchChecksums(deps, cfg)
		common.PrintStats(storageStats)
		printResults(storageResults, *sampleSize)

		outPath := filepath.Join(outputDir, "storage-checksums.json")
		if err := writeJSON(outPath, storageResults, storageStats); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing Storage JSON: %v\n", err)
		} else {
			fmt.Printf("\n  Storage API results written to: %s\n", outPath)
		}
	}

	if *mode == "both" {
		common.PrintComparison(aqlStats, storageStats)
	}
}

type jsonOutput struct {
	WorkingDir   string                    `json:"workingDirectory"`
	Approach     string                    `json:"approach"`
	Stats        common.FetchStats         `json:"stats"`
	Dependencies []common.DependencyResult `json:"dependencies"`
}

func writeJSON(path string, results []common.DependencyResult, stats common.FetchStats) error {
	wd, _ := os.Getwd()
	out := jsonOutput{
		WorkingDir:   wd,
		Approach:     stats.Approach,
		Stats:        stats,
		Dependencies: results,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func printResults(results []common.DependencyResult, limit int) {
	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}

	fmt.Printf("\n  Sample results (first %d of %d):\n", limit, len(results))
	fmt.Printf("  %-35s %-10s %-15s %-12s %-12s %-20s\n",
		"Package", "Version", "Scope", "SHA256", "SHA1", "RequestedBy")
	fmt.Println("  " + repeat("-", 108))

	for i := 0; i < limit; i++ {
		r := results[i]
		sha256 := trunc(r.Checksums.SHA256, 10)
		sha1 := trunc(r.Checksums.SHA1, 10)
		reqBy := ""
		if len(r.RequestedBy) > 0 {
			reqBy = r.RequestedBy[0]
			if len(r.RequestedBy) > 1 {
				reqBy += fmt.Sprintf(" +%d", len(r.RequestedBy)-1)
			}
		}
		fmt.Printf("  %-35s %-10s %-15s %-12s %-12s %-20s\n",
			r.Name, r.Version, r.Scope, sha256, sha1, reqBy)
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + ".."
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
