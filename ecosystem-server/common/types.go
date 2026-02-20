package common

import "time"

type Checksums struct {
	SHA512 string `json:"sha512,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	SHA1   string `json:"sha1,omitempty"`
	MD5    string `json:"md5,omitempty"`
}

type Dependency struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	ResolvedURL string   `json:"resolvedUrl"`
	Repo        string   `json:"repo"`
	Scope       string   `json:"scope"`
	RequestedBy []string `json:"requestedBy"`
}

type DependencyResult struct {
	Dependency
	Checksums Checksums `json:"checksums"`
}

type PackageTiming struct {
	Name     string        `json:"name"`
	Version  string        `json:"version"`
	Duration time.Duration `json:"duration"`
}

type FetchStats struct {
	Approach       string          `json:"approach"`
	TotalDeps      int             `json:"totalDeps"`
	SuccessCount   int             `json:"successCount"`
	FailCount      int             `json:"failCount"`
	APICallCount   int             `json:"apiCallCount"`
	TotalTime      time.Duration   `json:"totalTime"`
	AvgPerPackage  time.Duration   `json:"avgPerPackage"`
	MinPerPackage  time.Duration   `json:"minPerPackage"`
	MaxPerPackage  time.Duration   `json:"maxPerPackage"`
	P50PerPackage  time.Duration   `json:"p50PerPackage"`
	P95PerPackage  time.Duration   `json:"p95PerPackage"`
	P99PerPackage  time.Duration   `json:"p99PerPackage"`
	PackageTimings []PackageTiming `json:"-"`
}

type Config struct {
	ArtifactoryURL   string
	ArtifactoryToken string
	Concurrency      int
	AQLBatchSize     int
	RetryMax         int
	TimeoutSeconds   int
}
