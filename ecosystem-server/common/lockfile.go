package common

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type pnpmLsEntry struct {
	Dependencies    map[string]pnpmLsDep `json:"dependencies"`
	DevDependencies map[string]pnpmLsDep `json:"devDependencies"`
}

type pnpmLsDep struct {
	From         string                `json:"from"`
	Version      string                `json:"version"`
	Resolved     string                `json:"resolved"`
	Path         string                `json:"path"`
	Dependencies map[string]pnpmLsDep  `json:"dependencies"`
}

// ParseDependencies runs "pnpm ls --depth Infinity --json" in the given project
// directory and returns a deduplicated list of all dependencies with resolved URLs.
func ParseDependencies(projectDir string) ([]Dependency, error) {
	cmd := exec.Command("pnpm", "ls", "--depth", "Infinity", "--json")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running pnpm ls: %w", err)
	}

	var entries []pnpmLsEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parsing pnpm ls output: %w", err)
	}

	depMap := make(map[string]*Dependency)

	for _, entry := range entries {
		walkDeps(entry.Dependencies, "dependencies", depMap)
		walkDeps(entry.DevDependencies, "devDependencies", depMap)
	}

	deps := make([]Dependency, 0, len(depMap))
	for _, d := range depMap {
		deps = append(deps, *d)
	}
	return deps, nil
}

// ParseDependenciesFromFile reads a pre-generated pnpm ls JSON file instead of
// running the command. Useful when pnpm is not available.
func ParseDependenciesFromFile(path string) ([]Dependency, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading pnpm ls file: %w", err)
	}

	var entries []pnpmLsEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing pnpm ls JSON: %w", err)
	}

	depMap := make(map[string]*Dependency)
	for _, entry := range entries {
		walkDeps(entry.Dependencies, "dependencies", depMap)
		walkDeps(entry.DevDependencies, "devDependencies", depMap)
	}

	deps := make([]Dependency, 0, len(depMap))
	for _, d := range depMap {
		deps = append(deps, *d)
	}
	return deps, nil
}

func walkDeps(deps map[string]pnpmLsDep, scope string, depMap map[string]*Dependency) {
	for name, info := range deps {
		walkSingle(name, info, scope, "root", depMap)
	}
}

func walkSingle(name string, info pnpmLsDep, scope, parentName string, depMap map[string]*Dependency) {
	key := name + "@" + info.Version
	if existing, ok := depMap[key]; ok {
		addParent(existing, parentName)
	} else {
		resolvedURL := info.Resolved
		if resolvedURL == "" {
			resolvedURL = buildDefaultResolvedURL(name, info.Version)
		}

		repo := ""
		if resolvedURL != "" {
			parts, err := ParseTarballURL(resolvedURL)
			if err == nil {
				repo = parts.Repo
			}
		}

		dep := &Dependency{
			Name:        name,
			Version:     info.Version,
			ResolvedURL: resolvedURL,
			Repo:        repo,
			Scope:       scope,
			RequestedBy: []string{parentName},
		}
		depMap[key] = dep
	}

	for childName, childInfo := range info.Dependencies {
		walkSingle(childName, childInfo, "transitive", name, depMap)
	}
}

func addParent(dep *Dependency, parent string) {
	for _, p := range dep.RequestedBy {
		if p == parent {
			return
		}
	}
	if len(dep.RequestedBy) < 3 {
		dep.RequestedBy = append(dep.RequestedBy, parent)
	}
}

func buildDefaultResolvedURL(name, version string) string {
	if name == "" || version == "" {
		return ""
	}
	return filepath.Join(name, "-", fmt.Sprintf("%s-%s.tgz", name, version))
}
