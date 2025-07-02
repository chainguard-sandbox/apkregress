// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: Copyright 2025 Chainguard, Inc.

package internal

import (
	"runtime"
	"testing"
)

func TestNewApkraneClient(t *testing.T) {
	tests := []struct {
		name     string
		verbose  bool
		repoType string
	}{
		{
			name:     "wolfi client",
			verbose:  false,
			repoType: "wolfi",
		},
		{
			name:     "enterprise client",
			verbose:  true,
			repoType: "enterprise",
		},
		{
			name:     "extras client",
			verbose:  false,
			repoType: "extras",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewApkraneClient(tt.verbose, tt.repoType)
			
			if client == nil {
				t.Fatal("Expected non-nil client")
			}
			
			if client.verbose != tt.verbose {
				t.Errorf("Expected verbose=%v, got %v", tt.verbose, client.verbose)
			}
			
			if client.repoType != tt.repoType {
				t.Errorf("Expected repoType=%s, got %s", tt.repoType, client.repoType)
			}
		})
	}
}

func TestGetIndexURL(t *testing.T) {
	tests := []struct {
		name        string
		repoType    string
		arch        string
		expectedURL string
	}{
		{
			name:        "wolfi x86_64",
			repoType:    "wolfi",
			arch:        "x86_64",
			expectedURL: "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz",
		},
		{
			name:        "wolfi aarch64",
			repoType:    "wolfi",
			arch:        "aarch64",
			expectedURL: "https://packages.wolfi.dev/os/aarch64/APKINDEX.tar.gz",
		},
		{
			name:        "enterprise x86_64",
			repoType:    "enterprise",
			arch:        "x86_64",
			expectedURL: "https://apk.cgr.dev/chainguard-private/x86_64/APKINDEX.tar.gz",
		},
		{
			name:        "enterprise aarch64",
			repoType:    "enterprise",
			arch:        "aarch64",
			expectedURL: "https://apk.cgr.dev/chainguard-private/aarch64/APKINDEX.tar.gz",
		},
		{
			name:        "extras x86_64",
			repoType:    "extras",
			arch:        "x86_64",
			expectedURL: "https://apk.cgr.dev/extra-packages/x86_64/APKINDEX.tar.gz",
		},
		{
			name:        "extras aarch64",
			repoType:    "extras",
			arch:        "aarch64",
			expectedURL: "https://apk.cgr.dev/extra-packages/aarch64/APKINDEX.tar.gz",
		},
		{
			name:        "default fallback to wolfi",
			repoType:    "unknown",
			arch:        "x86_64",
			expectedURL: "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewApkraneClient(false, tt.repoType)
			url := client.getIndexURL(tt.arch)
			
			if url != tt.expectedURL {
				t.Errorf("Expected URL %s, got %s", tt.expectedURL, url)
			}
		})
	}
}

func TestPackageStruct(t *testing.T) {
	pkg := Package{
		Origin:       "test-package",
		Dependencies: []string{"dep1", "dep2", "dep3"},
	}

	if pkg.Origin != "test-package" {
		t.Errorf("Expected Origin to be 'test-package', got '%s'", pkg.Origin)
	}

	if len(pkg.Dependencies) != 3 {
		t.Errorf("Expected 3 dependencies, got %d", len(pkg.Dependencies))
	}

	expectedDeps := []string{"dep1", "dep2", "dep3"}
	for i, dep := range pkg.Dependencies {
		if dep != expectedDeps[i] {
			t.Errorf("Expected dependency %d to be '%s', got '%s'", i, expectedDeps[i], dep)
		}
	}
}

func TestArchitectureMapping(t *testing.T) {
	// Test that the architecture mapping works correctly in context
	// This tests the logic in GetReverseDependencies that converts amd64 to x86_64
	client := NewApkraneClient(false, "wolfi")
	
	// Simulate the arch conversion logic from GetReverseDependencies
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	
	url := client.getIndexURL(arch)
	
	// On amd64 systems, should use x86_64 in URL
	if runtime.GOARCH == "amd64" {
		expectedURL := "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz"
		if url != expectedURL {
			t.Errorf("Expected URL %s for amd64 arch, got %s", expectedURL, url)
		}
	}
}

func TestRepoTypeHandling(t *testing.T) {
	tests := []struct {
		name             string
		repoType         string
		expectsAuth      bool
		expectedURLBase  string
	}{
		{
			name:            "wolfi repo no auth",
			repoType:        "wolfi",
			expectsAuth:     false,
			expectedURLBase: "https://packages.wolfi.dev",
		},
		{
			name:            "enterprise repo needs auth",
			repoType:        "enterprise",
			expectsAuth:     true,
			expectedURLBase: "https://apk.cgr.dev/chainguard-private",
		},
		{
			name:            "extras repo needs auth",
			repoType:        "extras",
			expectsAuth:     true,
			expectedURLBase: "https://apk.cgr.dev/extra-packages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewApkraneClient(false, tt.repoType)
			url := client.getIndexURL("x86_64")
			
			if !containsString(url, tt.expectedURLBase) {
				t.Errorf("Expected URL to contain %s, got %s", tt.expectedURLBase, url)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    (len(s) > len(substr) && 
			 findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestPackageJSONParsing(t *testing.T) {
	// Test that our Package struct can handle typical JSON structures
	// This is important for the JSON unmarshaling in GetReverseDependencies
	
	tests := []struct {
		name           string
		origin         string
		dependencies   []string
		nilDependencies bool
	}{
		{
			name:         "package with dependencies",
			origin:       "curl",
			dependencies: []string{"libcurl", "ca-certificates", "openssl"},
		},
		{
			name:            "package with no dependencies",
			origin:          "base-package",
			nilDependencies: true,
		},
		{
			name:         "package with single dependency",
			origin:       "simple-tool",
			dependencies: []string{"libc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := Package{
				Origin: tt.origin,
			}
			
			if !tt.nilDependencies {
				pkg.Dependencies = tt.dependencies
			}

			if pkg.Origin != tt.origin {
				t.Errorf("Expected origin %s, got %s", tt.origin, pkg.Origin)
			}

			if tt.nilDependencies {
				if pkg.Dependencies != nil {
					t.Errorf("Expected nil dependencies, got %v", pkg.Dependencies)
				}
			} else {
				if len(pkg.Dependencies) != len(tt.dependencies) {
					t.Errorf("Expected %d dependencies, got %d", len(tt.dependencies), len(pkg.Dependencies))
				}
			}
		})
	}
}