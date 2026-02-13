# Release Process

This document describes how to create a new release of BoatmanMode.

## Overview

BoatmanMode has **automatic versioning** enabled. Releases are created automatically when you push to `main`:

1. **Auto-Version Workflow** calculates the next version based on commit messages
2. **Release Workflow** builds binaries and creates a GitHub release
3. All artifacts are published automatically

### How Automatic Versioning Works

When you push to `main`, the auto-version workflow:

1. Reads your latest commit message
2. Determines the version bump type:
   - `breaking:` or `major:` → Major version bump (v2.0.0)
   - `feat:` or `feature:` or `minor:` → Minor version bump (v1.1.0)
   - Everything else → Patch version bump (v1.0.1)
3. Creates and pushes a new git tag (e.g., `v1.0.1`)
4. The new tag triggers the release workflow
5. Binaries are built and released automatically

**Example:**
```bash
# This will create v1.0.1 (patch bump)
git commit -m "fix: correct error handling in executor"
git push origin main

# This will create v1.1.0 (minor bump)
git commit -m "feat: add support for file-based prompts"
git push origin main

# This will create v2.0.0 (major bump)
git commit -m "breaking: change public API interface"
git push origin main
```

### What Gets Released

For each version tag, GitHub Actions will:

1. Run tests to ensure everything passes
2. Build binaries for multiple platforms (Linux, macOS, Windows)
3. Create checksums for all binaries
4. Generate a changelog from commit messages
5. Create a GitHub release with all artifacts
6. Upload binaries as release assets

## Supported Platforms

The release process builds binaries for:

- **Linux**: amd64, arm64
- **macOS (Darwin)**: amd64 (Intel), arm64 (Apple Silicon)
- **Windows**: amd64

## Prerequisites

Before creating a release:

1. **Ensure all tests pass:**
   ```bash
   go test ./...
   ```

2. **Update documentation** if needed (README.md, TASK_MODES.md, etc.)

3. **Review commit messages** since the last release - they will be used for the changelog

## Creating a Release

### Automatic Release (Recommended)

**Simply push to main** and a release will be created automatically:

```bash
# 1. Make your changes
git add .

# 2. Commit with appropriate prefix for version bump
git commit -m "feat: add new feature"  # → v1.1.0 (minor bump)
# OR
git commit -m "fix: fix bug"           # → v1.0.1 (patch bump)
# OR
git commit -m "breaking: major change" # → v2.0.0 (major bump)

# 3. Push to main
git push origin main

# That's it! A new version will be created automatically
```

The auto-version workflow will:
- Calculate the next version based on your commit message
- Create and push a git tag (e.g., `v1.1.0`)
- Trigger the release workflow
- Build and publish binaries

### Manual Release (Advanced)

If you prefer manual control, you can create tags yourself:

#### 1. Decide on Version Number

Follow [Semantic Versioning](https://semver.org/):

- **MAJOR** (v2.0.0): Incompatible API changes
- **MINOR** (v1.1.0): New features, backward compatible
- **PATCH** (v1.0.1): Bug fixes, backward compatible

#### 2. Create and Push a Tag

```bash
# Replace X.Y.Z with your version number
VERSION="v1.0.0"

# Create an annotated tag
git tag -a $VERSION -m "Release $VERSION"

# Push the tag to GitHub
git push origin $VERSION
```

#### 3. Monitor the Release

1. Go to your repository on GitHub
2. Click "Actions" tab
3. You should see the "Release" workflow running
4. Wait for it to complete (usually 2-5 minutes)

### 4. Verify the Release

1. Go to the "Releases" page on GitHub
2. Find your new release
3. Verify:
   - ✅ Release notes are generated
   - ✅ All platform binaries are attached
   - ✅ Checksums file is present
   - ✅ Archive files include documentation

### 5. Test a Binary

Download and test one of the release binaries:

```bash
# Example: Download macOS ARM64 binary
curl -LO https://github.com/philjestin/boatmanmode/releases/download/v1.0.0/boatmanmode_v1.0.0_Darwin_arm64.tar.gz

# Extract
tar -xzf boatmanmode_v1.0.0_Darwin_arm64.tar.gz

# Run
./boatman version
```

## Release Workflow Details

### What Triggers a Release?

Releases can be triggered in two ways:

1. **Automatic** (recommended): Push to `main` → Auto-version creates tag → Release triggered
2. **Manual**: Push a tag starting with `v` → Release triggered

```bash
# Automatic (push to main)
git push origin main        # ✅ Auto-creates tag, triggers release

# Manual (push tag)
git push origin v1.0.0      # ✅ Triggers release
git push origin v2.0.0-beta # ✅ Triggers release (marked as pre-release)
git push origin test-tag    # ❌ Does not trigger release (no 'v' prefix)
```

### Auto-Version Behavior

The auto-version workflow:
- **Triggers on**: Pushes to `main` branch
- **Skips**: Changes to documentation only (*.md, docs/, examples/)
- **Creates**: Annotated git tag with calculated version
- **Pushes**: Tag to origin, which triggers release workflow

To bypass auto-versioning for a specific push, include `[skip ci]` in your commit message:
```bash
git commit -m "docs: update README [skip ci]"
```

### Changelog Generation

The changelog is automatically generated from commit messages using [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` → "New Features" section
- `fix:` → "Bug Fixes" section
- `perf:` → "Performance Improvements" section
- `refactor:` → "Refactors" section
- Other commits → "Other Changes" section

**Example commit messages:**
```bash
git commit -m "feat: add support for file-based prompts"
git commit -m "fix: handle empty task descriptions"
git commit -m "perf: optimize git diff generation"
```

### Binary Naming Convention

Binaries follow this pattern:
```
boatmanmode_v{VERSION}_{OS}_{ARCH}.{tar.gz|zip}
```

Examples:
- `boatmanmode_v1.0.0_Linux_x86_64.tar.gz`
- `boatmanmode_v1.0.0_Darwin_arm64.tar.gz`
- `boatmanmode_v1.0.0_Windows_x86_64.zip`

## Using Released Binaries

### Manual Download

Users can download binaries from the [Releases page](https://github.com/philjestin/boatmanmode/releases):

```bash
# Example for macOS ARM64
VERSION="v1.0.0"
curl -LO https://github.com/philjestin/boatmanmode/releases/download/${VERSION}/boatmanmode_${VERSION}_Darwin_arm64.tar.gz
tar -xzf boatmanmode_${VERSION}_Darwin_arm64.tar.gz
sudo mv boatman /usr/local/bin/
```

### Install with Go

Users can install directly using Go:

```bash
go install github.com/philjestin/boatmanmode/cmd/boatman@v1.0.0
```

### Verify Download Integrity

Users can verify downloads using checksums:

```bash
# Download the checksums file
curl -LO https://github.com/philjestin/boatmanmode/releases/download/v1.0.0/checksums.txt

# Verify (on Linux/macOS)
sha256sum -c checksums.txt --ignore-missing

# Or manually check
sha256sum boatmanmode_v1.0.0_Darwin_arm64.tar.gz
```

## Pre-releases and Beta Versions

To create a pre-release:

```bash
# Use a version with a pre-release identifier
git tag -a v1.0.0-beta.1 -m "Beta release v1.0.0-beta.1"
git push origin v1.0.0-beta.1
```

GoReleaser will automatically mark releases as "pre-release" if the version contains:
- `-alpha`
- `-beta`
- `-rc`

## Troubleshooting

### Disable Automatic Versioning

If you prefer manual versioning only:

1. Delete or disable the auto-version workflow:
   ```bash
   rm .github/workflows/auto-version.yml
   # OR rename it
   mv .github/workflows/auto-version.yml .github/workflows/auto-version.yml.disabled
   ```

2. Commit and push the change

3. From now on, create releases manually by pushing tags

### Release Workflow Failed

1. Check the GitHub Actions logs for errors
2. Common issues:
   - **Tests failing**: Fix the tests and create a new tag
   - **Build errors**: Check that code compiles on all platforms
   - **Permission errors**: Ensure GITHUB_TOKEN has correct permissions

### Auto-Version Created Wrong Version

If auto-version created the wrong version bump:

1. Delete the tag:
   ```bash
   # Delete locally
   git tag -d v1.0.1

   # Delete on GitHub
   git push origin :refs/tags/v1.0.1

   # Delete the release on GitHub (via web UI)
   ```

2. Create the correct tag manually:
   ```bash
   git tag -a v1.1.0 -m "Release v1.1.0"
   git push origin v1.1.0
   ```

### Need to Redo a Release

If you need to redo a release:

```bash
# Delete the tag locally
git tag -d v1.0.0

# Delete the tag on GitHub
git push origin :refs/tags/v1.0.0

# Delete the release on GitHub (via web UI)
# Then recreate the tag and push again
```

### Testing Release Locally

Test the release process locally before pushing:

```bash
# Install GoReleaser
brew install goreleaser/tap/goreleaser

# Create a test release (doesn't publish)
goreleaser release --snapshot --clean

# Check the dist/ folder for built binaries
ls -lh dist/
```

## Configuration Files

The release process is configured by:

- `.github/workflows/auto-version.yml` - Auto-version workflow (creates tags from commits)
- `.github/workflows/release.yml` - Release workflow (builds and publishes)
- `.goreleaser.yml` - GoReleaser configuration
- `cmd/boatman/main.go` - Version variable definitions
- `internal/cli/version.go` - Version command implementation

## Future Enhancements

Potential additions to the release process:

1. **Homebrew Tap**: Auto-update a Homebrew formula for easy installation
   ```bash
   brew install philjestin/tap/boatman
   ```

2. **Docker Images**: Build and publish Docker images
   ```bash
   docker pull philjestin/boatmanmode:latest
   ```

3. **Chocolatey Package**: Windows package manager support

4. **AUR Package**: Arch Linux user repository

To enable these, uncomment the relevant sections in `.goreleaser.yml`.

## Best Practices

1. **Test thoroughly** before tagging a release
2. **Use semantic versioning** consistently
3. **Write clear commit messages** for better changelogs
4. **Document breaking changes** in commit messages and release notes
5. **Keep main branch stable** - only tag releases from main
6. **Update CHANGELOG.md** manually for major releases if needed

## Example Release Checklist

- [ ] All tests pass (`go test ./...`)
- [ ] Documentation is up to date
- [ ] Version number chosen (semantic versioning)
- [ ] Commit messages are clear since last release
- [ ] Tag created and pushed
- [ ] GitHub Actions workflow completed successfully
- [ ] Release appears on GitHub with all artifacts
- [ ] Downloaded and tested at least one binary
- [ ] Announced release (if applicable)
