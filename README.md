# Brewhouse

Brewery management app (HTMX + Go WASM GUI, Go HTTP API, SQLite).

## Prerequisites

- Go 1.26+

## Run locally

From `app/src`:

```powershell
# Build the WASM client
$env:GOOS = "js"
$env:GOARCH = "wasm"
go build -o internal/web/static/wasm/app.wasm ./cmd/wasm
Remove-Item Env:GOOS
Remove-Item Env:GOARCH

# Copy Go's WASM support script (once, or after upgrading Go)
Copy-Item "$(go env GOROOT)/lib/wasm/wasm_exec.js" internal/web/static/js/wasm_exec.js

# Start the HTTP server (default :8080)
go run ./cmd
```

Open http://localhost:8080 — splash loads, then the login page. On first start the default **admin** account is created and its generated password is shown on the login page until the first successful sign-in (save it securely).

### Environment

| Variable        | Default                              | Description                          |
|-----------------|--------------------------------------|--------------------------------------|
| `PORT`          | `8080`                               | HTTP listen port                     |
| `APP_VERSION`   | `dev`                                | App/build version                    |
| `DATABASE_PATH` | `brewhouse.db`                       | SQLite database file path            |
| `JWT_SECRET`    | `brewhouse-dev-secret-change-me`     | HMAC secret for JWTs (dev default)   |

Override `JWT_SECRET` in any real deployment. Passwords are stored as bcrypt hashes, never plaintext.
