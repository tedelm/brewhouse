# Brewhouse

Brewery management app (HTMX + Go WASM GUI, Go HTTP API, SQLite).

## Prerequisites

- Go 1.26+

## Run locally

From `app/src`:

```powershell
# Optional: local config (copy once, then edit)
Copy-Item .env.example .env

# Build WASM + server with version from VERSION baked into the binary
.\scripts\build.ps1

# Or for a quick server-only run (reads VERSION file at startup; rebuild WASM separately if needed)
$env:GOOS = "js"; $env:GOARCH = "wasm"
go build -o internal/web/static/wasm/app.wasm ./cmd/wasm
Remove-Item Env:GOOS; Remove-Item Env:GOARCH
Copy-Item "$(go env GOROOT)/lib/wasm/wasm_exec.js" internal/web/static/js/wasm_exec.js
go run ./cmd
```

Open http://localhost:8080 — splash loads, then the login page. On first start the default **admin** account is created and its generated password is shown on the login page until the first successful sign-in (save it securely).

### App version

Bump [`VERSION`](app/src/VERSION) when releasing. Production builds (`.\scripts\build.ps1`) inject it into the binary with `-ldflags`. The splash UI, shell, and `/api/version` (WASM stale-reload) all use that value. Azure does not need an app-settings version — the deployed binary already carries it.

Optional override: set `APP_VERSION` in the environment if you must force a version without rebuilding (takes effect only when the binary still has the placeholder `dev`).

### Environment

Configuration is read from process environment variables. For local development, copy `.env.example` to `.env` in `app/src` (or the repo root); the app loads `.env` only to fill keys that are not already set. On Azure Web App (or any host), set the same names as **Application settings** — no `.env` file is required, and host env vars always win.

| Variable        | Default                              | Description                          |
|-----------------|--------------------------------------|--------------------------------------|
| `PORT`          | `8080`                               | HTTP listen port                     |
| `DATABASE_PATH` | `brewhouse.db`                       | SQLite database file path            |
| `BACKUP_DIR`    | `backups`                            | Directory for SQLite backup files    |
| `JWT_SECRET`    | `brewhouse-dev-secret-change-me`     | HMAC secret for JWTs (dev default)   |
| `APP_VERSION`   | (from `VERSION` / ldflags)           | Optional override when binary is `dev` |

Set a strong `JWT_SECRET` in any real deployment (including Azure). Passwords are stored as bcrypt hashes, never plaintext.

### Install as app (PWA)

Brewhouse is installable from Chrome/Edge when served over **HTTPS** or **localhost**: use the browser’s Install / “Add to Home Screen” option after the service worker registers. Icons and the web app manifest live under `/static/`. Bumping `VERSION` and rebuilding updates the service worker cache name so clients pick up a new shell.
