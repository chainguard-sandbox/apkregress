// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: Copyright 2025 Chainguard, Inc.

package internal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

type ApkraneClient struct {
	verbose  bool
	repoType string
}

type Package struct {
	Origin       string   `json:"Origin"`
	Dependencies []string `json:"Dependencies"`
}

func NewApkraneClient(verbose bool, repoType string) *ApkraneClient {
	return &ApkraneClient{
		verbose:  verbose,
		repoType: repoType,
	}
}

func (a *ApkraneClient) getIndexURL(arch string) string {
	switch a.repoType {
	case "enterprise":
		return fmt.Sprintf("https://apk.cgr.dev/chainguard-private/%s/APKINDEX.tar.gz", arch)
	case "extras":
		return fmt.Sprintf("https://apk.cgr.dev/extra-packages/%s/APKINDEX.tar.gz", arch)
	default: // "wolfi"
		return fmt.Sprintf("https://packages.wolfi.dev/os/%s/APKINDEX.tar.gz", arch)
	}
}

func (a *ApkraneClient) setupAuth(cmd *exec.Cmd) error {
	// Get authentication token using chainctl
	tokenCmd := exec.Command("chainctl", "auth", "token", "--audience", "apk.cgr.dev")
	tokenOutput, err := tokenCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get authentication token: %w", err)
	}

	token := strings.TrimSpace(string(tokenOutput))
	httpAuth := fmt.Sprintf("basic:apk.cgr.dev:user:%s", token)

	// Set environment variable for the command
	cmd.Env = append(os.Environ(), fmt.Sprintf("HTTP_AUTH=%s", httpAuth))

	if a.verbose {
		fmt.Printf("Setting up authentication for %s repository\n", a.repoType)
	}

	return nil
}

func (a *ApkraneClient) GetReverseDependencies(packageName string) ([]string, error) {
	if a.verbose {
		fmt.Printf("Finding reverse dependencies for package: %s\n", packageName)
	}

	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	} else if arch == "arm64" {
		arch = "aarch64"
	}

	indexURL := a.getIndexURL(arch)

	cmd := exec.Command("apkrane", "ls", "--json", "--latest", indexURL)

	// Set up authentication for enterprise and extras repositories
	if a.repoType == "enterprise" || a.repoType == "extras" {
		if err := a.setupAuth(cmd); err != nil {
			return nil, fmt.Errorf("failed to setup authentication: %w", err)
		}
	}
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run apkrane ls for %s: %w", indexURL, err)
	}

	var packages []Package
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var pkg Package
		if err := json.Unmarshal([]byte(line), &pkg); err != nil {
			if a.verbose {
				fmt.Printf("Warning: failed to parse JSON line: %s\n", err)
			}
			continue
		}
		packages = append(packages, pkg)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read apkrane output: %w", err)
	}

	originSet := make(map[string]bool)
	for _, pkg := range packages {
		if pkg.Dependencies == nil {
			continue
		}

		for _, dep := range pkg.Dependencies {
			if strings.Contains(dep, packageName) {
				if pkg.Origin != "" {
					originSet[pkg.Origin] = true
				}
			}
		}
	}

	var origins []string
	for origin := range originSet {
		origins = append(origins, origin)
	}
	sort.Strings(origins)

	if a.verbose {
		fmt.Printf("Found %d reverse dependencies\n", len(origins))
	}

	return origins, nil
}
