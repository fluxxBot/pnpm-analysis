package common

import (
	"fmt"
	"net/url"
	"strings"
)

type TarballParts struct {
	Repo     string
	DirPath  string // e.g. "axios/-" or "@babel/core/-"
	FileName string // e.g. "axios-1.13.2.tgz" or "core-7.26.0.tgz"
	FullPath string // DirPath + "/" + FileName
}

// ParseTarballURL extracts repo, directory path, and filename from an Artifactory npm tarball URL.
//
// Input:  https://artifactory.example.com/api/npm/npm-remote/axios/-/axios-1.13.2.tgz
// Output: repo="npm-remote", dirPath="axios/-", fileName="axios-1.13.2.tgz"
//
// Input:  https://artifactory.example.com/api/npm/npm-remote/@babel/core/-/core-7.26.0.tgz
// Output: repo="npm-remote", dirPath="@babel/core/-", fileName="core-7.26.0.tgz"
func ParseTarballURL(tarballURL string) (TarballParts, error) {
	u, err := url.Parse(tarballURL)
	if err != nil {
		return TarballParts{}, fmt.Errorf("invalid tarball URL %q: %w", tarballURL, err)
	}

	path := strings.TrimPrefix(u.Path, "/")

	const apiNpmPrefix = "api/npm/"
	idx := strings.Index(path, apiNpmPrefix)
	if idx != -1 {
		path = path[idx+len(apiNpmPrefix):]
	}

	// path is now: <repo>/<package-path>/-/<filename>.tgz
	// Find repo name (first segment)
	slashIdx := strings.Index(path, "/")
	if slashIdx == -1 {
		return TarballParts{}, fmt.Errorf("cannot extract repo from path %q", path)
	}

	repo := path[:slashIdx]
	rest := path[slashIdx+1:] // e.g. "axios/-/axios-1.13.2.tgz" or "@babel/core/-/core-7.26.0.tgz"

	dashIdx := strings.Index(rest, "/-/")
	if dashIdx == -1 {
		return TarballParts{}, fmt.Errorf("cannot find /-/ separator in %q", rest)
	}

	dirPath := rest[:dashIdx] + "/-"
	fileName := rest[dashIdx+3:]
	fullPath := dirPath + "/" + fileName

	return TarballParts{
		Repo:     repo,
		DirPath:  dirPath,
		FileName: fileName,
		FullPath: fullPath,
	}, nil
}

// BuildTarballPartsFromName constructs TarballParts from package name + version
// when no tarball URL is available.
func BuildTarballPartsFromName(name, version string) TarballParts {
	var dirPath, fileName string

	if strings.HasPrefix(name, "@") {
		parts := strings.SplitN(name, "/", 2)
		if len(parts) == 2 {
			dirPath = name + "/-"
			fileName = parts[1] + "-" + version + ".tgz"
		}
	} else {
		dirPath = name + "/-"
		fileName = name + "-" + version + ".tgz"
	}

	return TarballParts{
		DirPath:  dirPath,
		FileName: fileName,
		FullPath: dirPath + "/" + fileName,
	}
}

// StorageAPIPath returns the full path for the Artifactory Storage API call.
// e.g. /api/storage/npm-remote/axios/-/axios-1.13.2.tgz
func StorageAPIPath(repo string, parts TarballParts) string {
	return fmt.Sprintf("/api/storage/%s/%s", repo, parts.FullPath)
}
