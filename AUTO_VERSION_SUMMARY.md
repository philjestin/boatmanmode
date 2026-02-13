# Automatic Versioning Summary

## What Changed

Added automatic version creation whenever code is pushed to the `main` branch. No more manual tagging required!

## How It Works

### Before (Manual)
```bash
# Had to manually create and push tags
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
# Wait for release...
```

### After (Automatic) ✨
```bash
# Just push to main with a descriptive commit message
git commit -m "feat: add new feature"
git push origin main
# Auto-version creates tag → Release builds automatically!
```

## Version Bump Rules

The auto-version workflow determines version bumps based on your commit message prefix:

| Commit Prefix | Version Bump | Example |
|--------------|--------------|---------|
| `breaking:` or `major:` | Major (X.0.0) | `v1.0.0` → `v2.0.0` |
| `feat:` or `feature:` or `minor:` | Minor (x.Y.0) | `v1.0.0` → `v1.1.0` |
| Everything else | Patch (x.y.Z) | `v1.0.0` → `v1.0.1` |

## Examples

```bash
# Patch version bump (bug fixes, docs, etc.)
git commit -m "fix: correct error handling in executor"
git push origin main
# Creates: v1.0.1

# Minor version bump (new features)
git commit -m "feat: add support for Jira tickets"
git push origin main
# Creates: v1.1.0

# Major version bump (breaking changes)
git commit -m "breaking: change public API interface"
git push origin main
# Creates: v2.0.0
```

## What Happens Automatically

When you push to `main`:

1. **Auto-Version Workflow** runs
   - Reads your commit message
   - Calculates next version
   - Creates git tag (e.g., `v1.0.1`)
   - Pushes tag to GitHub

2. **Release Workflow** triggers (from new tag)
   - Runs tests
   - Builds binaries for all platforms
   - Creates GitHub release
   - Uploads artifacts

3. **Release Published** 🎉
   - Binaries available for download
   - Changelog generated from commits
   - Users can install via `go get` or download

## Skipping Auto-Versioning

To skip versioning for a specific push (e.g., documentation-only changes):

```bash
git commit -m "docs: update README [skip ci]"
git push origin main
# No version created
```

Or, the workflow automatically skips these changes:
- `*.md` files (documentation)
- `docs/` directory
- `.github/` directory (workflows, templates)
- `examples/` directory

## Manual Versioning Still Works

You can still create versions manually if needed:

```bash
git tag -a v1.5.0 -m "Release v1.5.0"
git push origin v1.5.0
# Release workflow triggers normally
```

## Disabling Auto-Versioning

If you prefer manual versioning only:

```bash
# Delete the auto-version workflow
rm .github/workflows/auto-version.yml
git commit -m "disable auto-versioning"
git push origin main
```

## Troubleshooting

### Wrong version was created

Delete the tag and create manually:
```bash
# Delete locally
git tag -d v1.0.1

# Delete on GitHub
git push origin :refs/tags/v1.0.1

# Delete release on GitHub (via web UI)

# Create correct version
git tag -a v1.1.0 -m "Release v1.1.0"
git push origin v1.1.0
```

### Tag already exists

If auto-version tries to create a tag that exists, it will skip and do nothing. Check the Actions log for details.

### No release was created

Possible reasons:
1. Commit message included `[skip ci]`
2. Only documentation files changed
3. Tag already exists
4. Check GitHub Actions logs for errors

## Files Added/Modified

**Created:**
- `.github/workflows/auto-version.yml` - Auto-version workflow

**Modified:**
- `RELEASING.md` - Updated documentation
- `README.md` - Added note about automatic releases

## Benefits

✅ **Faster releases** - No manual tagging needed
✅ **Consistent versioning** - Follows semantic versioning automatically
✅ **Less friction** - Just push and forget
✅ **Clear intent** - Commit messages drive versioning
✅ **Still flexible** - Manual tags still work

## Best Practices

1. **Write clear commit messages** with appropriate prefixes
2. **Use conventional commits** for better changelogs
3. **Test before pushing to main** (releases are automatic!)
4. **Use feature branches** for work in progress
5. **Review commits** before merging to main

## Migration Path

If you have an existing repository:

1. Current tags remain unchanged
2. Next push to main will increment from latest tag
3. If no tags exist, starts at `v0.0.1`
4. All existing workflows continue to work

## Example Workflow

```bash
# 1. Create feature branch
git checkout -b feature/add-auth

# 2. Make changes
# ... code changes ...

# 3. Commit with feat prefix
git commit -m "feat: add JWT authentication support"

# 4. Push feature branch for review
git push origin feature/add-auth

# 5. Create PR, get approval, merge to main
# ... via GitHub UI or gh CLI ...

# 6. Release happens automatically!
# Auto-version creates v1.1.0 → Release builds → Binaries published
```

That's it! No manual tagging needed.
