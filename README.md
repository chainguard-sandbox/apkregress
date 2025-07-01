# apkregress

A Go-based tool that uses apkrane to generate a list of reverse dependencies of a provided package and then uses melange to run the test makefile target for each package against a provided APK repository. If a package test fails, it repeats the test without the provided APK repository to detect regressions.

## Features

- Find reverse dependencies using apkrane
- Test packages using melange with configurable concurrency
- Regression detection by comparing results with and without APK repository
- Verbose output option for detailed logging

## Requirements

- Go 1.21+
- [apkrane](https://github.com/jonjohnsonjr/apkrane) command-line tool
- `make` command
- For enterprise and extras repositories: `chainctl` command-line tool for authentication
- Access to one of the supported repositories:
  - wolfi-dev/os
  - chainguard-dev/enterprise-packages
  - chainguard-dev/extra-packages

## Installation

```bash
go build -o apkregress .
```

## Usage

```bash
./apkregress \
  --package <package-name> \
  --repo <apk-repository-url> \
  --repo-path <path-to-package-repo> \
  --repo-type <wolfi|enterprise|extras> \
  --concurrency 4 \
  --verbose
```

### Options

- `--package, -p`: Package name to find reverse dependencies for (required)
- `--package-file, -f`: File containing list of package names (one per line)
- `--repo, -r`: APK repository URL to test against (required)
- `--repo-path, -w`: Path to package repository (required)
- `--repo-type, -t`: Repository type: wolfi, enterprise, or extras (default: wolfi)
- `--concurrency, -c`: Number of concurrent test jobs (default: 4)
- `--verbose, -v`: Enable verbose output
- `--markdown, -m`: Output test summary in markdown format for GitHub issues
- `--hang-timeout`: Timeout for hung tests (default: 30m)

### Examples

#### Wolfi Repository
```bash
./apkregress \
  --package openssl \
  --repo https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz \
  --repo-path /path/to/wolfi-dev/os \
  --repo-type wolfi \
  --concurrency 8 \
  --verbose
```

#### Enterprise Repository
```bash
# Requires chainctl authentication
./apkregress \
  --package openssl \
  --repo https://apk.cgr.dev/chainguard-private/x86_64/APKINDEX.tar.gz \
  --repo-path /path/to/chainguard-dev/enterprise-packages \
  --repo-type enterprise \
  --concurrency 8 \
  --verbose
```

#### Extras Repository
```bash
# Requires chainctl authentication
./apkregress \
  --package openssl \
  --repo https://packages.cgr.dev/extras/x86_64/APKINDEX.tar.gz \
  --repo-path /path/to/chainguard-dev/extra-packages \
  --repo-type extras \
  --concurrency 8 \
  --verbose
```

## How it works

1. Uses apkrane to query the specified package index (Wolfi, Enterprise, or Extras) and find reverse dependencies
2. For each reverse dependency, runs two tests:
   - With the provided APK repository (using `MELANGE_EXTRA_OPTS`)
   - Without the provided APK repository
3. Compares results to detect regressions:
   - ✅ Pass: Both tests succeed or test improves with repository
   - ❌ Fail: Both tests fail (not a regression)
   - 🔴 Regression: Test fails with repository but passes without

## Output

The tool provides a summary showing:
- Total packages tested
- Number of regressions detected
- Successful and failed packages
- List of packages with regressions

Additionally, detailed result lists are written to the logs directory:
- `successful.txt`: Packages that passed all tests
- `failed.txt`: Packages that failed consistently
- `regressions.txt`: Packages showing regressions
- `hung.txt`: Packages that exceeded timeout
- `skipped.txt`: Packages that were skipped

Use `--markdown` flag to output the summary in markdown format suitable for GitHub issues.

Exit code 1 indicates regressions were found.
