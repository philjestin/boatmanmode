# BoatmanMode 🚣

An AI-powered development agent that automates ticket execution with peer review. BoatmanMode fetches tickets from Linear, generates code using Claude, reviews changes with ScottBott (a peer-review AI skill), iterates until quality passes, and creates pull requests.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            BoatmanMode Orchestrator                          │
│                                                                               │
│  ┌─────────────┐    ┌─────────────────────────────────────────────────────┐ │
│  │   Linear    │───▶│                   Workflow Engine                    │ │
│  │  (tickets)  │    │                                                       │ │
│  └─────────────┘    │  1. Fetch ticket                                      │ │
│                     │  2. Create git worktree                               │ │
│                     │  3. Execute task (Claude)                             │ │
│                     │  4. Review (ScottBott)                                │ │
│                     │  5. Refactor loop until pass                          │ │
│                     │  6. Create PR (gh CLI)                                │ │
│                     └─────────────────────────────────────────────────────┘ │
│                                        │                                      │
│            ┌───────────────────────────┼───────────────────────────┐         │
│            ▼                           ▼                           ▼         │
│  ┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐    │
│  │ tmux: executor  │       │ tmux: reviewer-1│       │ tmux: refactor-1│    │
│  │ ┌─────────────┐ │       │ ┌─────────────┐ │       │ ┌─────────────┐ │    │
│  │ │   Claude    │ │       │ │  ScottBott  │ │       │ │   Claude    │ │    │
│  │ │  (coding)   │ │       │ │  (review)   │ │       │ │ (refactor)  │ │    │
│  │ └─────────────┘ │       │ └─────────────┘ │       │ └─────────────┘ │    │
│  └─────────────────┘       └─────────────────┘       └─────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
                              ┌─────────────────┐
                              │     GitHub      │
                              │   (PR via gh)   │
                              └─────────────────┘
```

## Key Features

### 🤖 AI-Powered Development
- Generates complete implementations from ticket descriptions
- Understands project conventions and patterns
- Creates appropriate tests alongside code

### 👀 ScottBott Peer Review
- Automated code review with pass/fail verdict
- Identifies critical, major, and minor issues
- Provides actionable feedback for improvements
- Enforces quality standards before PR creation

### 🔄 Iterative Refinement
- Automatically refactors based on review feedback
- Each iteration uses a fresh agent (clean context)
- Configurable max iterations (default: 3)

### 📺 Observable Agents (tmux Sessions)
- Each agent runs in its own tmux session
- Watch agents work in real-time
- Debug by attaching to any session
- Full visibility into AI decision-making

### 🌲 Git Worktree Isolation
- Each ticket works in an isolated worktree
- No interference with your main working directory
- Parallel ticket execution possible
- Clean branch management

## Prerequisites

BoatmanMode leverages your existing authenticated CLI tools:

| Tool | Purpose | How to Authenticate |
|------|---------|---------------------|
| `claude` | AI code generation & review | `gcloud auth login` (Vertex AI) |
| `gh` | Pull request creation | `gh auth login` |
| `git` | Version control | SSH keys or credential helper |
| `tmux` | Agent session management | (no auth needed) |

### Claude CLI Setup

If using Vertex AI (Google Cloud):

```bash
# Authenticate with Google Cloud
gcloud auth login
gcloud auth application-default login

# Set environment variables (or use an alias)
export CLAUDE_CODE_USE_VERTEX=1
export CLOUD_ML_REGION=us-east5
export ANTHROPIC_VERTEX_PROJECT_ID=your-project-id
```

## Installation

### From Source

```bash
git clone https://github.com/handshake/boatmanmode
cd boatmanmode
go build -o boatman ./cmd/boatman

# Optional: Add to PATH
sudo mv boatman /usr/local/bin/
```

### Go Install

```bash
go install github.com/handshake/boatmanmode/cmd/boatman@latest
```

## Configuration

### Required: Linear API Key

```bash
export LINEAR_API_KEY=lin_api_xxxxx
```

Get your API key from: Linear Settings → API → Personal API Keys

### Optional: Config File

Create `~/.boatman.yaml` or `.boatman.yaml` in your project:

```yaml
# Linear API key (can also use LINEAR_API_KEY env var)
linear_key: lin_api_xxxxx

# Workflow settings
max_iterations: 3      # Max review/refactor cycles
base_branch: main      # Base branch for new worktrees
auto_pr: true          # Automatically create PR on success
```

## Usage

### Execute a Ticket

```bash
# Navigate to your project repo
cd /path/to/your/project

# Run boatman with a Linear ticket ID
boatman work ENG-123
```

### Watch Agents Work

```bash
# In another terminal, watch the active agent
boatman watch

# Or attach to a specific session
tmux attach -t boatman-executor
tmux attach -t boatman-reviewer-1
tmux attach -t boatman-refactor-1
```

**tmux controls:**
- `Ctrl+B` then `D` - Detach (return to your terminal)
- `Ctrl+B` then arrow keys - Switch panes (if multiple)

### Manage Sessions

```bash
# List all active boatman sessions
boatman sessions list

# Kill all sessions
boatman sessions kill

# Kill a specific session
boatman sessions kill boatman-executor

# Clean up idle sessions
boatman sessions cleanup
```

### Command Options

```bash
boatman work ENG-123 --max-iterations 5    # More refactor attempts
boatman work ENG-123 --base-branch develop # Use different base branch
boatman work ENG-123 --dry-run             # Preview without changes
```

## Workflow Details

### Step 1: Fetch Ticket
Retrieves ticket details from Linear including title, description, labels, and suggested branch name.

### Step 2: Create Worktree
Creates an isolated git worktree at `.worktrees/<branch-name>/` based on the latest main branch.

### Step 3: Execute Task
Spawns a Claude agent (`boatman-executor`) that:
- Analyzes the ticket requirements
- Plans the implementation
- Generates code files with complete contents

### Step 4: Code Review
Spawns ScottBott (`boatman-reviewer-N`) that:
- Reviews the diff against ticket requirements
- Scores the implementation (0-100)
- Identifies issues by severity (critical/major/minor)
- Makes a pass/fail decision

**Pass Criteria:**
- No critical issues
- No more than 2 major issues
- Code accomplishes ticket requirements
- Code follows project conventions

### Step 5: Refactor (if needed)
If review fails, spawns a refactor agent (`boatman-refactor-N`) that:
- Receives the review feedback
- Reads the current implementation
- Applies fixes and improvements
- Stages changes for re-review

This loops until review passes or max iterations reached.

### Step 6: Create PR
On success:
- Commits changes with conventional commit message
- Pushes branch to origin
- Creates PR via `gh pr create`

## Writing Effective Tickets

BoatmanMode works best with detailed tickets. Include:

### 1. Clear Requirements
```markdown
## Requirements
- Create POST /api/auth/login endpoint
- Accept email and password in request body
- Return JWT token on success
- Return 401 on invalid credentials
```

### 2. Technical Context
```markdown
## Technical Context
- Use existing User model in internal/models
- JWT secret is in config.JWTSecret
- Follow existing handler patterns in internal/api
```

### 3. Acceptance Criteria
```markdown
## Acceptance Criteria
- [ ] Endpoint validates input
- [ ] Password is checked against bcrypt hash
- [ ] JWT includes user ID and expiration
- [ ] Unit tests cover success and failure cases
```

### 4. Constraints
```markdown
## Constraints
- Do not modify existing endpoints
- Must be backward compatible
- Performance: < 100ms response time
```

## Project Structure

```
boatmanmode/
├── cmd/
│   └── boatman/
│       └── main.go           # CLI entry point
├── internal/
│   ├── agent/
│   │   └── agent.go          # Workflow orchestration
│   ├── claude/
│   │   └── claude.go         # Claude CLI wrapper
│   ├── cli/
│   │   ├── root.go           # Cobra root command
│   │   ├── work.go           # work command
│   │   └── sessions.go       # sessions/watch commands
│   ├── config/
│   │   └── config.go         # Configuration management
│   ├── executor/
│   │   └── executor.go       # Code generation agent
│   ├── github/
│   │   └── github.go         # PR creation via gh CLI
│   ├── linear/
│   │   └── client.go         # Linear API client
│   ├── scottbott/
│   │   └── scottbott.go      # Peer review agent
│   ├── tmux/
│   │   └── tmux.go           # tmux session management
│   └── worktree/
│       └── worktree.go       # Git worktree management
├── go.mod
├── go.sum
└── README.md
```

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `LINEAR_API_KEY` | Linear API key for fetching tickets | Yes |
| `CLAUDE_CODE_USE_VERTEX` | Set to `1` for Vertex AI | If using Vertex |
| `CLOUD_ML_REGION` | Vertex AI region (e.g., `us-east5`) | If using Vertex |
| `ANTHROPIC_VERTEX_PROJECT_ID` | Google Cloud project ID | If using Vertex |
| `BOATMAN_DEBUG` | Set to `1` for debug output | No |

## Troubleshooting

### "No files were extracted from response"
Claude didn't produce code in the expected format. Check:
- Is your ticket detailed enough?
- Run `boatman watch` to see what Claude is outputting
- Check if Claude is asking clarifying questions instead of coding

### "argument list too long"
The prompt exceeded shell limits. This is handled automatically by piping via stdin, but if you see this:
- Ensure you're on the latest boatman version
- Check that temp files are being created in `/tmp/boatman-sessions/`

### tmux session not found
```bash
# Check if tmux is running
tmux list-sessions

# Kill any stuck sessions
boatman sessions kill
```

### Claude authentication issues
```bash
# Test claude CLI directly
claude -p "Hello, respond with just 'OK'"

# Re-authenticate if needed
gcloud auth login
gcloud auth application-default login
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `go test ./...`
5. Submit a PR

## License

MIT

---

*Built with 🚣 by the Handshake team*
