# BoatmanMode 🚣

An AI-powered development agent that automates ticket execution with peer review. BoatmanMode fetches tickets from Linear, generates code using Claude, reviews changes with the `peer-review` skill, iterates until quality passes, and creates pull requests.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            BoatmanMode Orchestrator                          │
│                                                                               │
│  ┌─────────────┐    ┌─────────────────────────────────────────────────────┐ │
│  │   Linear    │───▶│                   Workflow Engine                    │ │
│  │  (tickets)  │    │                                                       │ │
│  └─────────────┘    │  1. Fetch ticket         4. Review (peer-review)     │ │
│                     │  2. Create worktree      5. Refactor loop            │ │
│                     │  3. Execute (Claude)     6. Create PR (gh)           │ │
│                     └─────────────────────────────────────────────────────┘ │
│                                        │                                      │
│            ┌───────────────────────────┼───────────────────────────┐         │
│            ▼                           ▼                           ▼         │
│  ┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐    │
│  │ tmux: executor  │       │ tmux: reviewer-1│       │ tmux: refactor-1│    │
│  │ ┌─────────────┐ │       │ ┌─────────────┐ │       │ ┌─────────────┐ │    │
│  │ │   Claude    │ │       │ │ peer-review │ │       │ │   Claude    │ │    │
│  │ │  (coding)   │ │       │ │   skill     │ │       │ │ (refactor)  │ │    │
│  │ └─────────────┘ │       │ └─────────────┘ │       │ └─────────────┘ │    │
│  └─────────────────┘       └─────────────────┘       └─────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Key Features

### 🤖 AI-Powered Development
- Generates complete implementations from ticket descriptions
- Understands project conventions via Claude's context
- Creates appropriate tests alongside code

### 👀 Peer Review with Claude Skill
- Uses the `peer-review` Claude skill from your repo
- Automated pass/fail verdict with detailed feedback
- Falls back to built-in review if skill not found

### 🔄 Iterative Refinement
- Automatically refactors based on review feedback
- Fresh agent per iteration (clean context, no token bloat)
- Structured handoffs between agents (concise context)

### 📺 Live Activity Streaming
- Watch Claude work in real-time via tmux
- See every tool call: file reads, edits, bash commands
- Full visibility into AI decision-making

### 🌲 Git Worktree Isolation
- Each ticket works in an isolated worktree
- No interference with your main working directory
- Commit and push changes at any time

## Prerequisites

| Tool | Purpose | How to Authenticate |
|------|---------|---------------------|
| `claude` | AI code generation & review | `gcloud auth login` (Vertex AI) |
| `gh` | Pull request creation | `gh auth login` |
| `git` | Version control | SSH keys or credential helper |
| `tmux` | Agent session management | (no auth needed) |

### Claude CLI Setup (Vertex AI)

```bash
# Authenticate with Google Cloud
gcloud auth login
gcloud auth application-default login

# Set environment (or use an alias)
export CLAUDE_CODE_USE_VERTEX=1
export CLOUD_ML_REGION=us-east5
export ANTHROPIC_VERTEX_PROJECT_ID=your-project-id
```

## Installation

```bash
git clone https://github.com/handshake/boatmanmode
cd boatmanmode
go build -o boatman ./cmd/boatman

# Optional: Add to PATH
sudo mv boatman /usr/local/bin/
```

## Configuration

### Required: Linear API Key

```bash
export LINEAR_API_KEY=lin_api_xxxxx
```

### Optional: Config File

Create `~/.boatman.yaml`:

```yaml
linear_key: lin_api_xxxxx
max_iterations: 3
base_branch: main
```

## Usage

### Execute a Ticket

```bash
cd /path/to/your/project
boatman work ENG-123
```

### Watch Claude Work (Live Streaming)

```bash
# In another terminal
boatman watch

# Or attach to specific session
tmux attach -t boatman-executor
tmux attach -t boatman-reviewer-1
```

**What you'll see:**
```
🤖 Claude is working (with file write permissions)...
📝 Activity will stream below:

🔧 Running: ls -la packs/
📖 Reading: packs/annotations/app/graphql/consumer/types/...
✏️  Editing: packs/annotations/app/graphql/consumer/mutations/...
📝 Writing: packs/annotations/spec/graphql/consumer/...
🔍 Searching files...

📊 Task completed!
```

**tmux controls:**
- `Ctrl+B` then `D` - Detach
- `Ctrl+B` then arrow keys - Switch panes

### Manage Sessions

```bash
boatman sessions list       # List active sessions
boatman sessions kill       # Kill all boatman sessions
boatman sessions kill -f    # Also kill orphaned claude processes
boatman sessions cleanup    # Clean up idle sessions
```

### Manage Worktrees

```bash
boatman worktree list                    # List all worktrees
boatman worktree commit                  # Commit changes (WIP)
boatman worktree commit wt-name "msg"    # Commit with message
boatman worktree push                    # Push branch to origin
boatman worktree clean                   # Remove all worktrees
```

### View Changes Manually

```bash
# Go to worktree
cd .worktrees/philmiddleton-eng-123-feature

# See changes
git status
git diff

# Commit and push
git add -A
git commit -m "feat: implement feature"
git push -u origin HEAD

# Or checkout in main repo
cd /path/to/project
git checkout philmiddleton/eng-123-feature
```

### Command Options

```bash
boatman work ENG-123 --max-iterations 5    # More refactor attempts
boatman work ENG-123 --base-branch develop # Different base branch
boatman work ENG-123 --dry-run             # Preview without changes
```

## Workflow Details

### Agent Sessions

Each phase spawns a fresh Claude agent in its own tmux session:

| Session | Purpose |
|---------|---------|
| `boatman-executor` | Initial code generation |
| `boatman-reviewer-1` | First code review |
| `boatman-refactor-1` | First refactor (if needed) |
| `boatman-reviewer-2` | Second review (if needed) |
| ... | Continues until pass or max iterations |

### Structured Handoffs

Agents receive concise, focused context:

- **Executor** → Full ticket description
- **Reviewer** → Requirements summary + diff + files changed
- **Refactor** → Numbered issue list + guidance + current code

This keeps token usage low and agents focused.

### Peer Review Skill

ScottBott tries to invoke the `peer-review` Claude skill:

```bash
claude -p --agent peer-review "review this diff..."
```

If the skill exists in your repo's `.claude/` directory, it's used. Otherwise, falls back to a built-in review prompt.

## How It Works

### Claude's Agentic Mode

BoatmanMode leverages Claude's **agentic mode** - Claude directly reads and writes files in the worktree using its built-in tools (Read, Edit, Write, Bash, Glob, Grep). After Claude completes, BoatmanMode detects what changed via `git status`.

**Security**: BoatmanMode runs Claude with `--dangerously-skip-permissions` to allow file writes without prompting. This is safe because:
- Claude only has access to the isolated worktree
- All changes are tracked via git
- You can review before committing/pushing

### Live Activity Streaming

BoatmanMode uses `--output-format stream-json` to capture Claude's tool usage in real-time. The stream is parsed to show human-readable activity:

| Icon | Meaning |
|------|---------|
| 🔧 | Running a bash command |
| 📖 | Reading a file |
| ✏️ | Editing a file |
| 📝 | Writing a new file |
| 🔍 | Searching files (glob/grep) |
| 💭 | Claude's thinking |
| 📊 | Task completed |

### File Change Detection

After Claude completes, BoatmanMode runs `git status --porcelain` to detect all modified, added, and deleted files. This is more reliable than parsing output since Claude writes files directly.

## Writing Effective Tickets

Include:

```markdown
## Requirements
- Clear, specific requirements
- Acceptance criteria

## Technical Context
- Relevant file paths
- Existing patterns to follow
- APIs to use

## Constraints
- What NOT to change
- Performance requirements
```

## Project Structure

```
boatmanmode/
├── cmd/boatman/main.go       # Entry point
├── internal/
│   ├── agent/                # Workflow orchestration
│   ├── claude/               # Claude CLI wrapper
│   ├── cli/                  # Cobra commands
│   ├── config/               # Configuration
│   ├── executor/             # Code generation
│   ├── github/               # PR creation (gh CLI)
│   ├── handoff/              # Agent context passing
│   ├── linear/               # Linear API client
│   ├── scottbott/            # Peer review
│   ├── tmux/                 # Session management
│   └── worktree/             # Git worktree management
└── README.md
```

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `LINEAR_API_KEY` | Linear API key | Yes |
| `CLAUDE_CODE_USE_VERTEX` | Set to `1` for Vertex AI | If using Vertex |
| `CLOUD_ML_REGION` | Vertex AI region | If using Vertex |
| `ANTHROPIC_VERTEX_PROJECT_ID` | GCP project ID | If using Vertex |
| `BOATMAN_DEBUG` | Set to `1` for debug output | No |

## Troubleshooting

### "No files were changed in the worktree"

Claude ran but didn't modify any files. Possible causes:
- Ticket too vague - add more specific requirements
- Implementation already exists - Claude may just be analyzing
- Run `boatman watch` to see what Claude was doing

### Claude seems stuck

Check if Claude is actually working:
```bash
boatman watch  # See live activity
```

If truly stuck, kill and restart:
```bash
boatman sessions kill --force
boatman work ENG-123
```

### Session not found

```bash
boatman sessions kill  # Kill stuck sessions
boatman sessions list  # Verify clean state
```

### Want to see changes before PR

```bash
boatman worktree list                    # Find the worktree
cd .worktrees/<name>                     # Go there
git diff                                 # See changes
boatman worktree commit                  # Commit them
```

### Timeout waiting for Claude

Large codebases take longer. The default timeout is 30 minutes. If Claude is actively working (visible in `boatman watch`), just wait. If stuck, use `boatman sessions kill --force`.

## License

MIT

---

*Built with 🚣 by the Handshake team*
