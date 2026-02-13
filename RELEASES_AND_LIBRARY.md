# GitHub Releases & Go Module Setup

## Summary

BoatmanMode is now set up for:
1. **Automated releases** with pre-built binaries for multiple platforms
2. **Go module** that can be used as a library dependency

## What Was Added

### GitHub Actions Workflows

#### 1. Test Workflow (`.github/workflows/test.yml`)
Runs on every push and PR:
- ✅ Tests on Go 1.22
- ✅ Runs go vet
- ✅ Executes all tests with race detection
- ✅ Uploads coverage to Codecov
- ✅ Builds the binary
- ✅ Runs golangci-lint

#### 2. Release Workflow (`.github/workflows/release.yml`)
Triggers on version tags (`v*`):
- 🚀 Builds binaries for multiple platforms
- 📦 Creates GitHub release
- ⬆️ Uploads release artifacts
- 📝 Generates changelog from commits

### GoReleaser Configuration (`.goreleaser.yml`)

Builds for:
- **Linux**: amd64, arm64
- **macOS**: amd64 (Intel), arm64 (Apple Silicon)
- **Windows**: amd64

Features:
- Compressed archives (tar.gz for Unix, zip for Windows)
- Checksum generation for verification
- Automatic changelog from conventional commits
- Includes documentation files in archives

### Version Support

#### Version Command (`internal/cli/version.go`)
```bash
boatman version                 # Simple output
boatman version --verbose       # Detailed info
```

Displays:
- Version number
- Git commit hash
- Build date
- Go version
- OS/Architecture

#### Version Variables (`cmd/boatman/main.go`)
GoReleaser injects build information at compile time:
- `version` - Git tag (e.g., v1.0.0)
- `commit` - Git commit hash
- `date` - Build timestamp
- `builtBy` - "goreleaser"

### Installation Script (`install.sh`)

User-friendly installation:
```bash
# Install latest version
curl -fsSL https://raw.githubusercontent.com/philjestin/boatmanmode/main/install.sh | bash

# Install specific version
curl -fsSL https://raw.githubusercontent.com/philjestin/boatmanmode/main/install.sh | bash -s -- --version v1.0.0

# Install to custom directory
curl -fsSL https://raw.githubusercontent.com/philjestin/boatmanmode/main/install.sh | bash -s -- --dir ~/bin
```

Features:
- Auto-detects OS and architecture
- Downloads from GitHub releases
- Verifies binary extraction
- Handles sudo for /usr/local/bin
- Validates installation

### Library Usage Documentation

#### `LIBRARY_USAGE.md`
Complete API documentation with:
- Installation instructions
- Quick start example
- All three task types (Linear, Prompt, File)
- Working with Task interface
- Configuration options
- Multiple integration examples
- Best practices

#### `examples/simple/`
Working example showing:
- Basic library usage
- Creating tasks from prompts
- Executing workflows
- Handling results

### Release Documentation (`RELEASING.md`)

Complete guide for:
- Creating releases
- Semantic versioning
- Changelog generation
- Testing releases locally
- Troubleshooting
- Best practices

## How to Use

### As a CLI Tool

#### Installation

**Pre-built binary:**
```bash
curl -fsSL https://raw.githubusercontent.com/philjestin/boatmanmode/main/install.sh | bash
```

**With Go:**
```bash
go install github.com/philjestin/boatmanmode/cmd/boatman@latest
```

**From source:**
```bash
git clone https://github.com/philjestin/boatmanmode
cd boatmanmode
go build -o boatman ./cmd/boatman
sudo mv boatman /usr/local/bin/
```

#### Usage
```bash
boatman version
boatman work ENG-123
boatman work --prompt "Add authentication"
boatman work --file ./task.txt
```

### As a Go Library

#### Installation
```bash
go get github.com/philjestin/boatmanmode@latest
```

#### Basic Usage
```go
package main

import (
    "context"
    "github.com/philjestin/boatmanmode/internal/agent"
    "github.com/philjestin/boatmanmode/internal/config"
    "github.com/philjestin/boatmanmode/internal/task"
)

func main() {
    cfg := &config.Config{
        LinearKey:     "your-key",
        BaseBranch:    "main",
        MaxIterations: 3,
        EnableTools:   true,
    }

    a, _ := agent.New(cfg)
    t, _ := task.CreateFromPrompt("Add health check", "", "")
    result, _ := a.Work(context.Background(), t)

    if result.PRCreated {
        println("PR:", result.PRURL)
    }
}
```

## Creating Your First Release

### 1. Prepare

```bash
# Ensure all tests pass
go test ./...

# Build locally
go build ./cmd/boatman

# Test the binary
./boatman version
```

### 2. Create and Push Tag

```bash
# Choose version number (semantic versioning)
VERSION="v1.0.0"

# Create annotated tag
git tag -a $VERSION -m "Release $VERSION"

# Push to GitHub
git push origin $VERSION
```

### 3. Monitor Release

1. Go to GitHub Actions tab
2. Watch "Release" workflow
3. Wait for completion (~2-5 minutes)

### 4. Verify Release

1. Check GitHub Releases page
2. Verify all platform binaries are attached
3. Download and test a binary:

```bash
# Download
curl -LO https://github.com/philjestin/boatmanmode/releases/download/v1.0.0/boatmanmode_v1.0.0_Darwin_arm64.tar.gz

# Extract
tar -xzf boatmanmode_v1.0.0_Darwin_arm64.tar.gz

# Test
./boatman version
```

## Release Artifacts

Each release includes:

### Binaries
- `boatmanmode_v{VERSION}_Linux_x86_64.tar.gz`
- `boatmanmode_v{VERSION}_Linux_arm64.tar.gz`
- `boatmanmode_v{VERSION}_Darwin_x86_64.tar.gz`
- `boatmanmode_v{VERSION}_Darwin_arm64.tar.gz`
- `boatmanmode_v{VERSION}_Windows_x86_64.zip`

### Additional Files
- `checksums.txt` - SHA256 checksums for verification
- Each archive includes: binary, README.md, LICENSE, TASK_MODES.md

## Changelog Format

The changelog is auto-generated from commit messages using [Conventional Commits](https://www.conventionalcommits.org/):

### Commit Message Format
```
<type>: <description>

<optional body>
```

### Types
- `feat:` → "New Features" section
- `fix:` → "Bug Fixes" section
- `perf:` → "Performance Improvements" section
- `refactor:` → "Refactors" section
- Other → "Other Changes" section

### Examples
```bash
git commit -m "feat: add support for file-based prompts"
git commit -m "fix: handle empty task descriptions correctly"
git commit -m "perf: optimize git diff generation"
```

## Version Numbering

Follow [Semantic Versioning](https://semver.org/):

- **MAJOR (v2.0.0)**: Breaking API changes
- **MINOR (v1.1.0)**: New features, backward compatible
- **PATCH (v1.0.1)**: Bug fixes, backward compatible

### Examples
- First release: `v1.0.0`
- Add new feature: `v1.1.0`
- Fix bug: `v1.0.1`
- Breaking change: `v2.0.0`

## Go Module Versions

Users can install specific versions:

```bash
# Latest version
go get github.com/philjestin/boatmanmode@latest

# Specific version
go get github.com/philjestin/boatmanmode@v1.0.0

# Specific commit
go get github.com/philjestin/boatmanmode@abc123
```

## Testing Releases Locally

Before pushing a tag, test the release process locally:

```bash
# Install GoReleaser
brew install goreleaser/tap/goreleaser

# Create test release (doesn't publish)
goreleaser release --snapshot --clean

# Check output
ls -lh dist/

# Test a binary
./dist/boatmanmode_darwin_arm64/boatman version
```

## Binary Verification

Users can verify downloads:

```bash
# Download checksums
curl -LO https://github.com/philjestin/boatmanmode/releases/download/v1.0.0/checksums.txt

# Verify (Linux/macOS)
sha256sum -c checksums.txt --ignore-missing

# Or manual check
sha256sum boatmanmode_v1.0.0_Darwin_arm64.tar.gz
```

## Troubleshooting

### Release Workflow Failed

Check GitHub Actions logs:
- Tests must pass before release
- Code must compile on all platforms
- Tag must follow `v*` format

### Binary Won't Run

**Permission error:**
```bash
chmod +x boatman
```

**Not in PATH:**
```bash
sudo mv boatman /usr/local/bin/
```

**Wrong architecture:**
Download correct binary for your platform:
```bash
uname -m  # Shows architecture (x86_64, arm64, etc.)
```

## Future Enhancements

Potential additions to release process:

### 1. Homebrew Tap
```bash
brew install philjestin/tap/boatman
```

Uncomment in `.goreleaser.yml`:
```yaml
brews:
  - name: boatman
    repository:
      owner: philjestin
      name: homebrew-tap
```

### 2. Docker Images
```bash
docker pull philjestin/boatmanmode:latest
```

Uncomment in `.goreleaser.yml`:
```yaml
dockers:
  - image_templates:
      - "philjestin/boatmanmode:{{ .Tag }}"
```

### 3. Package Managers
- **Chocolatey** (Windows)
- **AUR** (Arch Linux)
- **Snap** (Universal Linux)

## Documentation

- **Release Process**: See `RELEASING.md`
- **Library Usage**: See `LIBRARY_USAGE.md`
- **CLI Usage**: See `README.md`
- **Task Modes**: See `TASK_MODES.md`
- **Examples**: See `examples/`

## Support

- **Issues**: [GitHub Issues](https://github.com/philjestin/boatmanmode/issues)
- **Releases**: [GitHub Releases](https://github.com/philjestin/boatmanmode/releases)
- **Discussions**: [GitHub Discussions](https://github.com/philjestin/boatmanmode/discussions)
