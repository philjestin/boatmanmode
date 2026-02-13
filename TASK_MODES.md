# Multiple Input Modes for BoatmanMode

BoatmanMode now supports three ways to specify work: Linear tickets, inline prompts, and file-based prompts.

## Input Modes

### 1. Linear Mode (Default)

Work with Linear tickets as before:

```bash
boatman work ENG-123
```

The same complete workflow applies:
- Fetches ticket details from Linear
- Creates git worktree
- Plans and executes the task
- Runs tests and reviews
- Creates a pull request with Linear ticket link

### 2. Prompt Mode

Provide an inline text prompt directly:

```bash
boatman work --prompt "Add user authentication with JWT tokens"
```

**Features:**
- Auto-generates unique task ID: `prompt-20260213-150405-abc123`
- Extracts title from prompt (checks for markdown headers or uses first line)
- Auto-generates branch name: `prompt-20260213-150405-abc123-add-user-authentication`
- Full description is the entire prompt text
- Same 9-step workflow as Linear mode

**Override auto-generation:**

```bash
boatman work --prompt "Add auth" --title "Authentication Feature" --branch-name "feature/auth"
```

### 3. File Mode

Read the task prompt from a file:

```bash
boatman work --file ./tasks/authentication.md
```

**Features:**
- Reads prompt from file (supports markdown, text, etc.)
- Same auto-generation as prompt mode
- Metadata includes file path for reference
- Useful for complex tasks with detailed requirements

**Example task file:**

```markdown
# Add health check endpoint

Add a simple HTTP health check endpoint at `/health` that returns:
- `status`: "healthy"
- `timestamp`: current time in ISO 8601 format
- `version`: application version

Follow existing API patterns in the codebase.
```

## Branch Naming

### Linear Mode
```
ENG-123-add-authentication
```
Format: `{ticket-id}-{sanitized-title}`

### Prompt/File Mode
```
prompt-20260213-150405-abc123-add-authentication
```
Format: `{task-id}-{sanitized-title}`

Branch names are automatically sanitized:
- Lowercase conversion
- Spaces → hyphens
- Special characters removed
- Limited to 30 characters (for the title portion)

## Pull Request Formatting

### Linear Mode PR
```markdown
## Add authentication

### Ticket
[ENG-123](https://linear.app/issue/ENG-123)

### Description
{ticket description}

### Changes
{review summary}

### Quality
- Review iterations: 2
- Tests: ✅ 15 passed
- Coverage: 85.3%
```

### Prompt/File Mode PR
```markdown
## Add authentication

### Task
Prompt-based task (prompt-20260213-150405-abc123)

### Description
{prompt text - truncated to 500 chars}

### Changes
{review summary}

### Quality
- Review iterations: 1
- Tests: ✅ 12 passed
- Coverage: 92.1%
```

## Task IDs

### Linear
- Uses ticket identifier: `ENG-123`
- Stable and human-readable

### Prompt/File
- Auto-generated: `prompt-20260213-150405-abc123`
- Format: `prompt-{YYYYMMDD-HHMMSS}-{6-char-hash}`
- Unique per invocation
- Timestamp-based for traceability

## CLI Flags

### Input Mode Flags
- `--prompt`: Treat argument as inline prompt text
- `--file`: Read prompt from file path
- **Mutually exclusive**: Cannot use both `--prompt` and `--file`

### Override Flags (Prompt/File Mode Only)
- `--title`: Override auto-extracted task title
- `--branch-name`: Override auto-generated branch name

### Existing Flags (All Modes)
- `--max-iterations`: Maximum review/refactor iterations (default: 3)
- `--base-branch`: Base branch for worktree (default: "main")
- `--auto-pr`: Automatically create PR on success (default: true)
- `--dry-run`: Run without making changes
- `--timeout`: Timeout in minutes for each Claude agent (default: 60)
- `--review-skill`: Claude skill for code review (default: "peer-review")

## Validation

### Error Cases

**Multiple modes specified:**
```bash
boatman work --prompt "..." --file ./task.txt
# Error: only one of --prompt or --file can be specified
```

**Override flags without prompt/file mode:**
```bash
boatman work ENG-123 --title "Custom Title"
# Error: --title can only be used with --prompt or --file
```

**Nonexistent file:**
```bash
boatman work --file ./missing.txt
# Error: task file does not exist: ./missing.txt
```

**Empty prompt:**
```bash
boatman work --prompt ""
# Error: prompt cannot be empty
```

## Examples

### Quick iteration without Linear ticket
```bash
boatman work --prompt "Add a /health endpoint that returns status and version"
```

### Complex task from file
```bash
cat > task.md <<EOF
# Refactor error handling

Update all API endpoints to use the new custom error types:
- ValidationError for input validation failures
- AuthorizationError for permission issues
- NotFoundError for missing resources

Ensure error responses follow the standard format with:
- error_code
- message
- details (optional)
EOF

boatman work --file task.md
```

### Custom branch name for feature work
```bash
boatman work --prompt "Add Redis caching layer" --branch-name "feature/redis-cache"
```

## Backward Compatibility

**100% backward compatible** - all existing commands work unchanged:

```bash
boatman work ENG-123                     # ✅ Works as before
boatman work ENG-456 --max-iterations 5  # ✅ Works as before
boatman work ENG-789 --dry-run           # ✅ Works as before
```

New flags are optional and only affect prompt/file mode behavior.

## Future Extensions

The task abstraction makes it easy to add more input sources:

```bash
# Potential future modes
boatman work --github-issue 123
boatman work --jira PROJ-456
boatman work ENG-123 --additional-context notes.txt
```

Each new source just needs to implement the `Task` interface:
```go
type Task interface {
    GetID() string
    GetTitle() string
    GetDescription() string
    GetBranchName() string
    GetLabels() []string
    GetMetadata() TaskMetadata
}
```
