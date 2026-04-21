# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

Go 1.26.2 is installed at `C:\Program Files\Go\bin\go.exe` and **not on PATH by default** in the user's Git Bash. Invoke via the full path, or rely on the PATH propagating in fresh shells after `winget` installs.

```bash
# Build (produces todo-go.exe on Windows)
"/c/Program Files/Go/bin/go.exe" build .

# Run without building
"/c/Program Files/Go/bin/go.exe" run . <subcommand>

# All tests (suite ~5s because bcrypt is deliberately slow)
"/c/Program Files/Go/bin/go.exe" test ./...

# Single package / test
"/c/Program Files/Go/bin/go.exe" test ./internal/task -run TestStore_AddWithLabels -v
```

**CI** (`.github/workflows/ci.yml`) runs on push/PR: `go mod tidy` check, `go vet`, `go build`, `go test -race -count=1`.

**Running the server locally:** `./todo-go.exe serve` binds `localhost:8080`. On Windows, **the running `.exe` locks itself** — you must `taskkill //F //IM todo-go.exe` before `go build` can overwrite it. A `todo-go.exe~` rename-artifact left behind means the overwrite happened while the old process was still running; it's in `.gitignore` and safe to delete.

**CLI is scoped per user** via `--user <name>` (env: `TODO_GO_USER`, default: `"default"`). The CLI bypasses authentication — local access is trusted. The HTTP server requires cookie-based auth for all `/api/*` except `/api/login`, `/api/register`, `/api/logout`.

## Architecture

Single Go binary serving three surfaces: CLI, HTTP JSON API, and embedded web UI (`//go:embed web` inside `internal/server`).

### Packages (all under `internal/`)

| Package | Responsibility |
|---|---|
| `task` | `Task` model, per-user `Store` (JSON on disk), `Manager` (caches Stores), sorting, filtering, path resolution, legacy migration |
| `user` | User credentials (bcrypt hashes) in `users.json`; `Store.Register` / `Authenticate` |
| `session` | In-memory `Manager` issuing random cookie tokens; `RequireAuth` middleware puts the username on request context |
| `server` | HTTP routing and handlers split by concern (`auth.go`, `tasks.go`, `labels.go`); embeds `web/` |
| `cli` | Cobra subcommands; each RunE opens its own per-user `task.Store` |

`main.go` is nine lines — `cli.NewRootCmd().Execute()`.

### Storage layout (on disk)

```
~/.todo-go/                              (override: TODO_GO_DATA)
├── users.json                           credentials, mode 0600
└── users/
    └── <username>/
        └── tasks.json                   that user's tasks + public labels
```

**Legacy migration:** if `~/.todo-go/tasks.json` exists at the time of the *first* registration, `task.MigrateLegacy` moves it into that user's folder. Later registrations don't migrate.

### Multi-user sharing model

A user marks some of their labels **public**. Any task they own tagged with any public label becomes visible (read-only) to all other users. Private labels: only owner sees.

This is implemented by `server.aggregatedTasks`:
1. Returns the current user's tasks (all of them).
2. For each other user, opens their Store, and includes tasks that have at least one label in their `PublicLabels` set.
3. Every task in the response is wrapped as `TaskView` with an `owner` field and `public` flag.

**Writes (`PATCH`, `POST`, `DELETE`, `POST /api/reorder`) always target the current user's own store.** Shared tasks are read-only by construction: the server never looks up another user's store for mutations, and the frontend hides edit controls on non-owner rows. Task IDs are **per-user**, not global — `/api/tasks/5` always means "the current user's task #5."

### Frontend

`internal/server/web/index.html` is a single-file vanilla-JS app. Interesting patterns:

- On load, `fetch("/api/me")` detects auth state; a 401 redirects to `/login`.
- All mutations call `refresh()` (full GET) after the PATCH/POST/DELETE — not optimistic.
- `Shift+click` on a label pill toggles public/private; right-click removes.
- Drag-to-reorder only works when all three are true: no label filter, `all` status filter, `manual` sort.

`login.html` and `register.html` post to `/api/login` and `/api/register`; on success the server sets the session cookie (`HttpOnly`, `SameSite=Strict`) and the client redirects to `/`.

### Env vars

- `TODO_GO_DATA` — override the `~/.todo-go` data directory (handy for tests — they all use `t.TempDir()`)
- `TODO_GO_USER` — CLI default user (overridden by `--user`)
- `TODO_GO_ADDR` — server bind address (overridden by `--addr`)

## Conventions in this repo

- **No comments.** Existing files are deliberately comment-free; identifiers carry the meaning. Only add comments for non-obvious WHY (hidden invariants, workarounds). Delete any `// what this does` style comment you encounter.
- **Test layout mirrors the package.** `internal/task/store_test.go` lives with `store.go`; same package, direct access to unexported helpers.
- **Windows line endings.** `git config core.autocrlf` is default-true on Git for Windows; you'll see `LF will be replaced by CRLF` warnings on commit — harmless, leave alone.
- **Commit discipline.** User-facing features and refactors go as separate commits. Each commit message leads with a short title and then a dash-list of what changed, grouped by layer (Store / CLI / API / Frontend).
