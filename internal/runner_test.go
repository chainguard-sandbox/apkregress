// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: Copyright 2025 Chainguard, Inc.

package internal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewRegressionTestRunner(t *testing.T) {
	tests := []struct {
		name           string
		packageName    string
		apkRepo        string
		repoPath       string
		repoType       string
		concurrency    int
		verbose        bool
		hangTimeout    time.Duration
		markdownOutput bool
	}{
		{
			name:           "basic runner",
			packageName:    "test-package",
			apkRepo:        "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz",
			repoPath:       "/tmp/packages",
			repoType:       "wolfi",
			concurrency:    4,
			verbose:        false,
			hangTimeout:    30 * time.Minute,
			markdownOutput: false,
		},
		{
			name:           "verbose runner with custom timeout",
			packageName:    "curl",
			apkRepo:        "https://apk.cgr.dev/chainguard-private/x86_64/APKINDEX.tar.gz",
			repoPath:       "/home/user/enterprise-packages",
			repoType:       "enterprise",
			concurrency:    8,
			verbose:        true,
			hangTimeout:    45 * time.Minute,
			markdownOutput: true,
		},
		{
			name:           "zero timeout defaults to 30 minutes",
			packageName:    "openssl",
			apkRepo:        "https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz",
			repoPath:       "/tmp/packages",
			repoType:       "wolfi",
			concurrency:    2,
			verbose:        false,
			hangTimeout:    0, // Should default to 30 minutes
			markdownOutput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewRegressionTestRunner(
				tt.packageName,
				tt.apkRepo,
				tt.repoPath,
				tt.repoType,
				tt.concurrency,
				tt.verbose,
				tt.hangTimeout,
				tt.markdownOutput,
			)

			if runner == nil {
				t.Fatal("Expected non-nil runner")
			}

			if runner.packageName != tt.packageName {
				t.Errorf("Expected packageName=%s, got %s", tt.packageName, runner.packageName)
			}

			if runner.apkRepo != tt.apkRepo {
				t.Errorf("Expected apkRepo=%s, got %s", tt.apkRepo, runner.apkRepo)
			}

			if runner.repoPath != tt.repoPath {
				t.Errorf("Expected repoPath=%s, got %s", tt.repoPath, runner.repoPath)
			}

			if runner.repoType != tt.repoType {
				t.Errorf("Expected repoType=%s, got %s", tt.repoType, runner.repoType)
			}

			if runner.concurrency != tt.concurrency {
				t.Errorf("Expected concurrency=%d, got %d", tt.concurrency, runner.concurrency)
			}

			if runner.verbose != tt.verbose {
				t.Errorf("Expected verbose=%v, got %v", tt.verbose, runner.verbose)
			}

			if runner.markdownOutput != tt.markdownOutput {
				t.Errorf("Expected markdownOutput=%v, got %v", tt.markdownOutput, runner.markdownOutput)
			}

			expectedTimeout := tt.hangTimeout
			if expectedTimeout == 0 {
				expectedTimeout = 30 * time.Minute
			}
			if runner.hangTimeout != expectedTimeout {
				t.Errorf("Expected hangTimeout=%v, got %v", expectedTimeout, runner.hangTimeout)
			}

			// Check that log directory contains package name
			if !strings.Contains(runner.logDir, tt.packageName) {
				t.Errorf("Expected logDir to contain package name, got %s", runner.logDir)
			}

			// Check that clients are initialized
			if runner.apkrane == nil {
				t.Error("Expected apkrane client to be initialized")
			}

			if runner.melange == nil {
				t.Error("Expected melange client to be initialized")
			}
		})
	}
}

func TestNewRegressionTestRunnerFromPackageList(t *testing.T) {
	packages := []string{"package1", "package2", "package3"}
	
	runner := NewRegressionTestRunnerFromPackageList(
		packages,
		"https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz",
		"/tmp/packages",
		"wolfi",
		4,
		false,
		30*time.Minute,
		false,
	)

	if runner == nil {
		t.Fatal("Expected non-nil runner")
	}

	expectedPackageName := "3 packages from file"
	if runner.packageName != expectedPackageName {
		t.Errorf("Expected packageName=%s, got %s", expectedPackageName, runner.packageName)
	}

	// Check that log directory contains "package-list-test"
	if !strings.Contains(runner.logDir, "package-list-test") {
		t.Errorf("Expected logDir to contain 'package-list-test', got %s", runner.logDir)
	}
}

func TestTestResult(t *testing.T) {
	tests := []struct {
		name     string
		result   TestResult
		expected TestResult
	}{
		{
			name: "successful test with repo",
			result: TestResult{
				Package:  "test-pkg",
				WithRepo: true,
				Success:  true,
				Error:    nil,
				Hung:     false,
				Skipped:  false,
			},
			expected: TestResult{
				Package:  "test-pkg",
				WithRepo: true,
				Success:  true,
				Error:    nil,
				Hung:     false,
				Skipped:  false,
			},
		},
		{
			name: "failed test with error",
			result: TestResult{
				Package:  "test-pkg",
				WithRepo: false,
				Success:  false,
				Error:    errors.New("test failed"),
				Hung:     false,
				Skipped:  false,
			},
			expected: TestResult{
				Package:  "test-pkg",
				WithRepo: false,
				Success:  false,
				Error:    errors.New("test failed"),
				Hung:     false,
				Skipped:  false,
			},
		},
		{
			name: "hung test",
			result: TestResult{
				Package:  "test-pkg",
				WithRepo: true,
				Success:  false,
				Error:    ErrTestHung,
				Hung:     true,
				Skipped:  false,
			},
			expected: TestResult{
				Package:  "test-pkg",
				WithRepo: true,
				Success:  false,
				Error:    ErrTestHung,
				Hung:     true,
				Skipped:  false,
			},
		},
		{
			name: "skipped test",
			result: TestResult{
				Package:  "test-pkg",
				WithRepo: true,
				Success:  false,
				Error:    ErrPackageYAMLNotFound,
				Hung:     false,
				Skipped:  true,
			},
			expected: TestResult{
				Package:  "test-pkg",
				WithRepo: true,
				Success:  false,
				Error:    ErrPackageYAMLNotFound,
				Hung:     false,
				Skipped:  true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.Package != tt.expected.Package {
				t.Errorf("Expected Package=%s, got %s", tt.expected.Package, tt.result.Package)
			}

			if tt.result.WithRepo != tt.expected.WithRepo {
				t.Errorf("Expected WithRepo=%v, got %v", tt.expected.WithRepo, tt.result.WithRepo)
			}

			if tt.result.Success != tt.expected.Success {
				t.Errorf("Expected Success=%v, got %v", tt.expected.Success, tt.result.Success)
			}

			if tt.result.Hung != tt.expected.Hung {
				t.Errorf("Expected Hung=%v, got %v", tt.expected.Hung, tt.result.Hung)
			}

			if tt.result.Skipped != tt.expected.Skipped {
				t.Errorf("Expected Skipped=%v, got %v", tt.expected.Skipped, tt.result.Skipped)
			}

			// For error comparison, check if both are nil or both contain the same message
			if (tt.result.Error == nil) != (tt.expected.Error == nil) {
				t.Errorf("Error nil mismatch: expected %v, got %v", tt.expected.Error, tt.result.Error)
			} else if tt.result.Error != nil && tt.expected.Error != nil {
				if tt.result.Error.Error() != tt.expected.Error.Error() {
					t.Errorf("Expected Error=%v, got %v", tt.expected.Error, tt.result.Error)
				}
			}
		})
	}
}


func TestLogDirectoryCreation(t *testing.T) {
	tests := []struct {
		name        string
		packageName string
		useFileMode bool
	}{
		{
			name:        "single package mode",
			packageName: "openssl",
			useFileMode: false,
		},
		{
			name:        "package list mode",
			packageName: "",
			useFileMode: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runner *RegressionTestRunner
			
			if tt.useFileMode {
				packages := []string{"pkg1", "pkg2"}
				runner = NewRegressionTestRunnerFromPackageList(
					packages,
					"https://example.com/repo",
					"/tmp",
					"wolfi",
					4,
					false,
					30*time.Minute,
					false,
				)
			} else {
				runner = NewRegressionTestRunner(
					tt.packageName,
					"https://example.com/repo",
					"/tmp",
					"wolfi",
					4,
					false,
					30*time.Minute,
					false,
				)
			}

			// Check that log directory path is set
			if runner.logDir == "" {
				t.Error("Expected logDir to be set")
			}

			// Check that log directory path contains timestamp pattern
			if !strings.Contains(runner.logDir, "logs/") {
				t.Errorf("Expected logDir to contain 'logs/', got %s", runner.logDir)
			}

			// Check the specific directory pattern based on mode
			if tt.useFileMode {
				if !strings.Contains(runner.logDir, "package-list-test-") {
					t.Errorf("Expected logDir to contain 'package-list-test-', got %s", runner.logDir)
				}
			} else {
				if !strings.Contains(runner.logDir, fmt.Sprintf("regression-test-%s-", tt.packageName)) {
					t.Errorf("Expected logDir to contain 'regression-test-%s-', got %s", tt.packageName, runner.logDir)
				}
			}
		})
	}
}

func TestProgressTracking(t *testing.T) {
	runner := &RegressionTestRunner{
		completedTests: 0,
		totalTests:     10,
		startTime:      time.Now().Add(-time.Minute), // 1 minute ago
		verbose:        false,
	}

	// Test progress update
	runner.updateProgress()
	
	if runner.completedTests != 1 {
		t.Errorf("Expected completedTests to be 1, got %d", runner.completedTests)
	}

	// Test multiple updates
	for i := 0; i < 5; i++ {
		runner.updateProgress()
	}
	
	if runner.completedTests != 6 {
		t.Errorf("Expected completedTests to be 6, got %d", runner.completedTests)
	}
}

func TestProgressTrackingVerboseMode(t *testing.T) {
	runner := &RegressionTestRunner{
		completedTests: 0,
		totalTests:     10,
		startTime:      time.Now(),
		verbose:        true, // In verbose mode, progress updates should be skipped
	}

	originalCompleted := runner.completedTests
	runner.updateProgress()
	
	// In verbose mode, completedTests should still be incremented
	// but no progress display should occur
	if runner.completedTests != originalCompleted+1 {
		t.Errorf("Expected completedTests to be incremented even in verbose mode")
	}
}

func TestProgressBoundaryConditions(t *testing.T) {
	tests := []struct {
		name           string
		completedTests int64
		totalTests     int64
		shouldUpdate   bool
	}{
		{
			name:           "normal progress",
			completedTests: 5,
			totalTests:     10,
			shouldUpdate:   true,
		},
		{
			name:           "completion",
			completedTests: 9, // Will become 10 after update
			totalTests:     10,
			shouldUpdate:   true,
		},
		{
			name:           "over completion",
			completedTests: 10,
			totalTests:     10,
			shouldUpdate:   false, // Should not update beyond total
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &RegressionTestRunner{
				completedTests: tt.completedTests,
				totalTests:     tt.totalTests,
				startTime:      time.Now(),
				verbose:        false,
			}

			originalCompleted := runner.completedTests
			runner.updateProgress()

			if tt.shouldUpdate {
				if runner.completedTests != originalCompleted+1 {
					t.Errorf("Expected completedTests to be incremented")
				}
			} else {
				if runner.completedTests != originalCompleted {
					t.Errorf("Expected completedTests to remain unchanged when over total")
				}
			}
		})
	}
}

func TestResultWriter(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "result_writer_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create ResultWriter
	rw, err := NewResultWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create ResultWriter: %v", err)
	}
	defer rw.Close()

	// Test writing to different result files
	testCases := []struct {
		filename    string
		packageName string
	}{
		{"successful.txt", "pkg1"},
		{"successful.txt", "pkg2"},
		{"failed.txt", "pkg3"},
		{"regressions.txt", "pkg4"},
		{"hung.txt", "pkg5"},
		{"skipped.txt", "pkg6"},
	}

	// Write test results
	for _, tc := range testCases {
		err := rw.WriteResult(tc.filename, tc.packageName)
		if err != nil {
			t.Errorf("Failed to write result to %s: %v", tc.filename, err)
		}
	}

	// Close writer to ensure all data is flushed
	rw.Close()

	// Verify file contents
	expectedContents := map[string][]string{
		"successful.txt":  {"pkg1", "pkg2"},
		"failed.txt":      {"pkg3"},
		"regressions.txt": {"pkg4"},
		"hung.txt":        {"pkg5"},
		"skipped.txt":     {"pkg6"},
	}

	for filename, expectedLines := range expectedContents {
		filePath := filepath.Join(tmpDir, filename)

		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", filename, err)
			continue
		}

		lines := strings.Split(strings.TrimSpace(string(content)), "\n")
		if !reflect.DeepEqual(lines, expectedLines) {
			t.Errorf("File %s content mismatch. Expected %v, got %v", filename, expectedLines, lines)
		}
	}
}

func TestWriteResultToFile(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "write_result_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create runner with ResultWriter
	runner := &RegressionTestRunner{
		logDir: tmpDir,
	}

	rw, err := NewResultWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create ResultWriter: %v", err)
	}
	defer rw.Close()
	runner.resultWriter = rw

	testCases := []struct {
		name             string
		packageName      string
		withRepoResult   TestResult
		withoutRepoResult *TestResult
		expectedFile     string
		expectedLine     string
	}{
		{
			name:        "successful test",
			packageName: "pkg1",
			withRepoResult: TestResult{
				Package: "pkg1",
				Success: true,
				Skipped: false,
				Hung:    false,
			},
			withoutRepoResult: nil,
			expectedFile:     "successful.txt",
			expectedLine:     "pkg1",
		},
		{
			name:        "skipped test",
			packageName: "pkg2",
			withRepoResult: TestResult{
				Package: "pkg2",
				Success: false,
				Skipped: true,
				Hung:    false,
			},
			withoutRepoResult: nil,
			expectedFile:     "skipped.txt",
			expectedLine:     "pkg2",
		},
		{
			name:        "regression detected",
			packageName: "pkg3",
			withRepoResult: TestResult{
				Package: "pkg3",
				Success: false,
				Skipped: false,
				Hung:    false,
			},
			withoutRepoResult: &TestResult{
				Package: "pkg3",
				Success: true,
				Skipped: false,
				Hung:    false,
			},
			expectedFile: "regressions.txt",
			expectedLine: "pkg3",
		},
		{
			name:        "failed in both scenarios",
			packageName: "pkg4",
			withRepoResult: TestResult{
				Package: "pkg4",
				Success: false,
				Skipped: false,
				Hung:    false,
			},
			withoutRepoResult: &TestResult{
				Package: "pkg4",
				Success: false,
				Skipped: false,
				Hung:    false,
			},
			expectedFile: "failed.txt",
			expectedLine: "pkg4",
		},
		{
			name:        "hung test with repo",
			packageName: "pkg5",
			withRepoResult: TestResult{
				Package: "pkg5",
				Success: false,
				Skipped: false,
				Hung:    true,
			},
			withoutRepoResult: nil,
			expectedFile:     "hung.txt",
			expectedLine:     "pkg5 (with repo)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runner.writeResultToFile(tc.packageName, &tc.withRepoResult, tc.withoutRepoResult)

			// Check that the expected file was written
			filePath := filepath.Join(tmpDir, tc.expectedFile)
			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Errorf("Failed to read file %s: %v", tc.expectedFile, err)
				return
			}

			lines := strings.Split(strings.TrimSpace(string(content)), "\n")
			found := false
			for _, line := range lines {
				if line == tc.expectedLine {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Expected line '%s' not found in file %s. Content: %v", tc.expectedLine, tc.expectedFile, lines)
			}
		})
	}
}