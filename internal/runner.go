// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: Copyright 2025 Chainguard, Inc.

package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"
)

type TestResult struct {
	Package  string
	WithRepo bool
	Success  bool
	Error    error
	Hung     bool
	Skipped  bool
}

type ResultWriter struct {
	mu        sync.Mutex
	files     map[string]*os.File
	logDir    string
}

func NewResultWriter(logDir string) (*ResultWriter, error) {
	rw := &ResultWriter{
		files:  make(map[string]*os.File),
		logDir: logDir,
	}

	// Create and open all result files for append
	filenames := []string{"successful.txt", "failed.txt", "regressions.txt", "hung.txt", "skipped.txt"}
	for _, filename := range filenames {
		filepath := filepath.Join(logDir, filename)
		file, err := os.OpenFile(filepath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			rw.Close() // Clean up already opened files
			return nil, fmt.Errorf("failed to open %s: %w", filename, err)
		}
		rw.files[filename] = file
	}

	return rw, nil
}

func (rw *ResultWriter) WriteResult(filename, packageName string) error {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	file, exists := rw.files[filename]
	if !exists {
		return fmt.Errorf("file %s not found", filename)
	}

	_, err := file.WriteString(packageName + "\n")
	if err != nil {
		return err
	}

	return file.Sync() // Ensure data is written to disk
}

func (rw *ResultWriter) Close() {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	for _, file := range rw.files {
		file.Close()
	}
}

func (r *RegressionTestRunner) writeResultToFile(packageName string, withRepoResult, withoutRepoResult *TestResult) {
	if r.resultWriter == nil {
		return // ResultWriter not initialized yet
	}

	// Determine result type and write immediately
	if withRepoResult.Skipped {
		r.resultWriter.WriteResult("skipped.txt", packageName)
		return
	}

	if withRepoResult.Hung {
		r.resultWriter.WriteResult("hung.txt", fmt.Sprintf("%s (with repo)", packageName))
		if withoutRepoResult != nil && withoutRepoResult.Hung {
			r.resultWriter.WriteResult("hung.txt", fmt.Sprintf("%s (without repo)", packageName))
		}
		return
	}

	if withoutRepoResult != nil && withoutRepoResult.Hung {
		r.resultWriter.WriteResult("hung.txt", fmt.Sprintf("%s (without repo)", packageName))
		return
	}

	// If with-repo test passed and no without-repo test was run
	if withRepoResult.Success && withoutRepoResult == nil {
		r.resultWriter.WriteResult("successful.txt", packageName)
		return
	}

	// If both tests were run because with-repo failed
	if !withRepoResult.Success && withoutRepoResult != nil {
		if withoutRepoResult.Success {
			// Regression: fails with repo, passes without
			r.resultWriter.WriteResult("regressions.txt", packageName)
		} else {
			// Failed in both scenarios
			r.resultWriter.WriteResult("failed.txt", packageName)
		}
		return
	}
}

type RegressionTestRunner struct {
	packageName    string
	apkRepo        string
	repoPath       string
	repoType       string
	concurrency    int
	verbose        bool
	logDir         string
	hangTimeout    time.Duration
	markdownOutput bool
	apkrane        *ApkraneClient
	melange        *MelangeClient
	completedTests int64
	totalTests     int64
	startTime      time.Time
	resultWriter   *ResultWriter
}

func (r *RegressionTestRunner) updateProgress() {
	// Check current value before incrementing
	current := atomic.LoadInt64(&r.completedTests)
	total := r.totalTests

	if current >= total {
		return // Already at or past completion
	}

	completed := atomic.AddInt64(&r.completedTests, 1)

	if r.verbose {
		return // Don't show progress in verbose mode
	}

	// Calculate progress percentage
	progress := float64(completed) / float64(total) * 100

	// Calculate elapsed time and estimate remaining time
	elapsed := time.Since(r.startTime)
	var eta time.Duration
	if completed > 0 {
		avgTimePerTest := elapsed / time.Duration(completed)
		remaining := total - completed
		eta = avgTimePerTest * time.Duration(remaining)
	}

	// Format the progress update
	if eta > 0 {
		fmt.Printf("\rProgress: %d/%d (%.1f%%) - ETA: %v", completed, total, progress, eta.Round(time.Second))
	} else {
		fmt.Printf("\rProgress: %d/%d (%.1f%%)", completed, total, progress)
	}

	// Print newline when complete
	if completed == total {
		fmt.Println()
	}
}

func NewRegressionTestRunner(packageName, apkRepo, repoPath, repoType string, concurrency int, verbose bool, hangTimeout time.Duration, markdownOutput bool) *RegressionTestRunner {
	// Create log directory with timestamp
	timestamp := time.Now().Format("20060102-150405")
	logDir := filepath.Join("logs", fmt.Sprintf("regression-test-%s-%s", packageName, timestamp))

	// Default to 30 minutes if no timeout specified
	if hangTimeout == 0 {
		hangTimeout = 30 * time.Minute
	}

	return &RegressionTestRunner{
		packageName:    packageName,
		apkRepo:        apkRepo,
		repoPath:       repoPath,
		repoType:       repoType,
		concurrency:    concurrency,
		verbose:        verbose,
		logDir:         logDir,
		hangTimeout:    hangTimeout,
		markdownOutput: markdownOutput,
		apkrane:        NewApkraneClient(verbose, repoType),
		melange:        NewMelangeClient(repoPath, verbose, logDir, hangTimeout),
		resultWriter:   nil, // Will be initialized in Run()
	}
}

func NewRegressionTestRunnerFromPackageList(packages []string, apkRepo, repoPath, repoType string, concurrency int, verbose bool, hangTimeout time.Duration, markdownOutput bool) *RegressionTestRunner {
	// Create log directory with timestamp
	timestamp := time.Now().Format("20060102-150405")
	logDir := filepath.Join("logs", fmt.Sprintf("package-list-test-%s", timestamp))

	// Default to 30 minutes if no timeout specified
	if hangTimeout == 0 {
		hangTimeout = 30 * time.Minute
	}

	return &RegressionTestRunner{
		packageName:    fmt.Sprintf("%d packages from file", len(packages)),
		apkRepo:        apkRepo,
		repoPath:       repoPath,
		repoType:       repoType,
		concurrency:    concurrency,
		verbose:        verbose,
		logDir:         logDir,
		hangTimeout:    hangTimeout,
		markdownOutput: markdownOutput,
		apkrane:        NewApkraneClient(verbose, repoType),
		melange:        NewMelangeClient(repoPath, verbose, logDir, hangTimeout),
		resultWriter:   nil, // Will be initialized in Run()
	}
}

func (r *RegressionTestRunner) Run() error {
	// Create log directory
	if err := os.MkdirAll(r.logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", r.logDir, err)
	}

	// Initialize result writer for real-time file updates
	var err error
	r.resultWriter, err = NewResultWriter(r.logDir)
	if err != nil {
		return fmt.Errorf("failed to create result writer: %w", err)
	}
	defer r.resultWriter.Close()

	reverseDeps, err := r.apkrane.GetReverseDependencies(r.packageName)
	if err != nil {
		return fmt.Errorf("failed to get reverse dependencies: %w", err)
	}

	if len(reverseDeps) == 0 {
		fmt.Printf("No reverse dependencies found for package: %s\n", r.packageName)
		return nil
	}

	fmt.Printf("Testing %d reverse dependencies with concurrency %d\n", len(reverseDeps), r.concurrency)
	fmt.Printf("Logs will be saved to: %s\n", r.logDir)

	// Initialize progress tracking
	r.totalTests = int64(len(reverseDeps))
	r.startTime = time.Now()

	results := make(chan TestResult, len(reverseDeps)*2)
	ctx := context.Background()
	sem := semaphore.NewWeighted(int64(r.concurrency))
	var wg sync.WaitGroup

	for _, pkg := range reverseDeps {
		wg.Add(1)
		go func(packageName string) {
			defer wg.Done()
			sem.Acquire(ctx, 1)
			defer sem.Release(1)

			// First test with repo
			err := r.melange.TestPackage(packageName, true, r.apkRepo)

			withRepoResult := TestResult{
				Package:  packageName,
				WithRepo: true,
				Success:  err == nil,
				Error:    err,
				Hung:     errors.Is(err, ErrTestHung),
				Skipped:  errors.Is(err, ErrPackageYAMLNotFound),
			}
			results <- withRepoResult

			var withoutRepoResult *TestResult

			// Only test without repo if test with repo failed and wasn't skipped
			if !withRepoResult.Success && !withRepoResult.Skipped {
				err := r.melange.TestPackage(packageName, false, r.apkRepo)

				// Skip if YAML file not found (shouldn't happen since we already checked, but for safety)
				if errors.Is(err, ErrPackageYAMLNotFound) {
					r.writeResultToFile(packageName, &withRepoResult, nil)
					r.updateProgress()
					return
				}

				withoutRepoTestResult := TestResult{
					Package:  packageName,
					WithRepo: false,
					Success:  err == nil,
					Error:    err,
					Hung:     errors.Is(err, ErrTestHung),
					Skipped:  errors.Is(err, ErrPackageYAMLNotFound),
				}
				withoutRepoResult = &withoutRepoTestResult
				results <- withoutRepoTestResult
			}

			// Write result to file immediately
			r.writeResultToFile(packageName, &withRepoResult, withoutRepoResult)

			// Update progress after completing all tests for this package
			r.updateProgress()
		}(pkg)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return r.analyzeResults(results, len(reverseDeps))
}

func (r *RegressionTestRunner) RunFromPackageList(packages []string) error {
	// Create log directory
	if err := os.MkdirAll(r.logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", r.logDir, err)
	}

	// Initialize result writer for real-time file updates
	var err error
	r.resultWriter, err = NewResultWriter(r.logDir)
	if err != nil {
		return fmt.Errorf("failed to create result writer: %w", err)
	}
	defer r.resultWriter.Close()

	if len(packages) == 0 {
		fmt.Println("No packages provided")
		return nil
	}

	fmt.Printf("Testing %d packages with concurrency %d\n", len(packages), r.concurrency)
	fmt.Printf("Logs will be saved to: %s\n", r.logDir)

	// Initialize progress tracking
	r.totalTests = int64(len(packages))
	r.startTime = time.Now()

	results := make(chan TestResult, len(packages)*2)
	ctx := context.Background()
	sem := semaphore.NewWeighted(int64(r.concurrency))
	var wg sync.WaitGroup

	for _, pkg := range packages {
		wg.Add(1)
		go func(packageName string) {
			defer wg.Done()
			sem.Acquire(ctx, 1)
			defer sem.Release(1)

			// First test with repo
			err := r.melange.TestPackage(packageName, true, r.apkRepo)

			withRepoResult := TestResult{
				Package:  packageName,
				WithRepo: true,
				Success:  err == nil,
				Error:    err,
				Hung:     errors.Is(err, ErrTestHung),
				Skipped:  errors.Is(err, ErrPackageYAMLNotFound),
			}
			results <- withRepoResult

			var withoutRepoResult *TestResult

			// Only test without repo if test with repo failed and wasn't skipped
			if !withRepoResult.Success && !withRepoResult.Skipped {
				err := r.melange.TestPackage(packageName, false, r.apkRepo)

				// Skip if YAML file not found (shouldn't happen since we already checked, but for safety)
				if errors.Is(err, ErrPackageYAMLNotFound) {
					r.writeResultToFile(packageName, &withRepoResult, nil)
					r.updateProgress()
					return
				}

				withoutRepoTestResult := TestResult{
					Package:  packageName,
					WithRepo: false,
					Success:  err == nil,
					Error:    err,
					Hung:     errors.Is(err, ErrTestHung),
					Skipped:  errors.Is(err, ErrPackageYAMLNotFound),
				}
				withoutRepoResult = &withoutRepoTestResult
				results <- withoutRepoTestResult
			}

			// Write result to file immediately
			r.writeResultToFile(packageName, &withRepoResult, withoutRepoResult)

			// Update progress after completing all tests for this package
			r.updateProgress()
		}(pkg)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return r.analyzeResults(results, len(packages))
}

func (r *RegressionTestRunner) analyzeResults(results chan TestResult, expectedPackages int) error {
	packageResults := make(map[string]map[bool]TestResult)

	for result := range results {
		if packageResults[result.Package] == nil {
			packageResults[result.Package] = make(map[bool]TestResult)
		}
		packageResults[result.Package][result.WithRepo] = result
	}

	var regressions []string
	var hungTests []string
	var successfulPackages []string
	var failedPackages []string
	var skippedPackages []string
	var successCount, failureCount, skippedCount int

	fmt.Println("\n=== Test Results ===")
	for pkg, results := range packageResults {
		withRepoResult, hasWithRepo := results[true]
		withoutRepoResult, hasWithoutRepo := results[false]

		if !hasWithRepo {
			fmt.Printf("⚠️  %s: Incomplete test results\n", pkg)
			continue
		}

		// Check for skipped tests first
		if withRepoResult.Skipped {
			skippedCount++
			skippedPackages = append(skippedPackages, pkg)
			if r.verbose {
				fmt.Printf("⏭️  %s: SKIPPED (YAML file not found)\n", pkg)
			}
			continue
		}

		// Check for hung tests
		if withRepoResult.Hung {
			hungTests = append(hungTests, fmt.Sprintf("%s (with repo)", pkg))
			fmt.Printf("⏰ %s: HUNG (with repo - killed after %v)\n", pkg, r.hangTimeout)
			if hasWithoutRepo && withoutRepoResult.Hung {
				hungTests = append(hungTests, fmt.Sprintf("%s (without repo)", pkg))
				fmt.Printf("⏰ %s: HUNG (without repo - killed after %v)\n", pkg, r.hangTimeout)
			}
			continue
		}
		if hasWithoutRepo && withoutRepoResult.Hung {
			hungTests = append(hungTests, fmt.Sprintf("%s (without repo)", pkg))
			fmt.Printf("⏰ %s: HUNG (without repo - killed after %v)\n", pkg, r.hangTimeout)
			continue
		}

		// If with-repo test passed, we didn't run without-repo test
		if withRepoResult.Success && !hasWithoutRepo {
			successCount++
			successfulPackages = append(successfulPackages, pkg)
			if r.verbose {
				fmt.Printf("✅ %s: PASS (with repo, without-repo test skipped)\n", pkg)
			}
		} else if !withRepoResult.Success && hasWithoutRepo {
			// Both tests were run because with-repo failed
			if withoutRepoResult.Success {
				regressions = append(regressions, pkg)
				fmt.Printf("🔴 %s: REGRESSION DETECTED (fails with repo, passes without)\n", pkg)
			} else {
				failureCount++
				failedPackages = append(failedPackages, pkg)
				if r.verbose {
					fmt.Printf("❌ %s: FAIL (both scenarios)\n", pkg)
				}
			}
		} else if !withRepoResult.Success && !hasWithoutRepo {
			fmt.Printf("⚠️  %s: Incomplete test results (with-repo failed but no without-repo test)\n", pkg)
			continue
		}
	}

	if r.markdownOutput {
		r.printMarkdownSummary(expectedPackages, skippedCount, len(packageResults)-skippedCount, len(regressions), len(hungTests), successCount, failureCount, regressions, hungTests)
	} else {
		fmt.Printf("\n=== Summary ===\n")
		fmt.Printf("Total packages found: %d\n", expectedPackages)
		fmt.Printf("Packages skipped (no YAML): %d\n", skippedCount)
		fmt.Printf("Packages tested: %d\n", len(packageResults)-skippedCount)
		fmt.Printf("Regressions detected: %d\n", len(regressions))
		fmt.Printf("Hung tests: %d\n", len(hungTests))
		fmt.Printf("Successful packages: %d\n", successCount)
		fmt.Printf("Failed packages: %d\n", failureCount)
	}

	if !r.markdownOutput {
		if len(hungTests) > 0 {
			fmt.Printf("\nTests that hung (killed after 30 minutes):\n")
			for _, test := range hungTests {
				fmt.Printf("  - %s\n", test)
			}
		}

		if len(regressions) > 0 {
			fmt.Printf("\nPackages with regressions:\n")
			for _, pkg := range regressions {
				fmt.Printf("  - %s\n", pkg)
			}
		}
	}

	if len(regressions) > 0 {
		return fmt.Errorf("found %d regressions", len(regressions))
	}

	if len(hungTests) > 0 {
		return fmt.Errorf("found %d hung tests", len(hungTests))
	}

	return nil
}

func (r *RegressionTestRunner) printMarkdownSummary(totalPackages, skippedCount, testedCount, regressionsCount, hungCount, successCount, failureCount int, regressions, hungTests []string) {
	fmt.Printf("\n## APK Regression Test Summary\n\n")
	fmt.Printf("**Package:** %s  \n", r.packageName)
	fmt.Printf("**APK Repository:** %s  \n", r.apkRepo)
	fmt.Printf("**Test Duration:** %v  \n\n", time.Since(r.startTime).Round(time.Second))

	fmt.Printf("### Test Results\n\n")
	fmt.Printf("| Metric | Count |\n")
	fmt.Printf("|--------|-------|\n")
	fmt.Printf("| Total packages found | %d |\n", totalPackages)
	fmt.Printf("| Packages skipped (no YAML) | %d |\n", skippedCount)
	fmt.Printf("| Packages tested | %d |\n", testedCount)
	fmt.Printf("| **Regressions detected** | **%d** |\n", regressionsCount)
	fmt.Printf("| Hung tests | %d |\n", hungCount)
	fmt.Printf("| Successful packages | %d |\n", successCount)
	fmt.Printf("| Failed packages | %d |\n", failureCount)

	if regressionsCount > 0 {
		fmt.Printf("\n### 🔴 Packages with Regressions\n\n")
		fmt.Printf("The following packages **fail with the new APK repository** but **pass without it**, indicating potential regressions:\n\n")
		for _, pkg := range regressions {
			fmt.Printf("- `%s`\n", pkg)
		}
	}

	if hungCount > 0 {
		fmt.Printf("\n### ⏰ Tests That Hung\n\n")
		fmt.Printf("The following tests were killed after %v timeout:\n\n", r.hangTimeout)
		for _, test := range hungTests {
			fmt.Printf("- `%s`\n", test)
		}
	}

	if regressionsCount == 0 && hungCount == 0 {
		fmt.Printf("\n### ✅ All Tests Passed\n\n")
		fmt.Printf("No regressions were detected. All packages either passed with the new repository or failed consistently in both scenarios.\n")
	}

	fmt.Printf("\n---\n")
	fmt.Printf("*Generated by apk-regression-test-runner*\n")
}

