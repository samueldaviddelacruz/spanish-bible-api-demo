# AGENTS.md

Single-package Go API (Huma v2 + chi + sqlx) serving Spanish RV1960 Bible data from a SQLite file. Everything lives in `main.go` + `CustomSchemaLinkTransformer.go`; no sub-packages, no frontend.

## Commands
- Run: `go run .` — serves on `:8888` (override via `PORT` env). Must run from repo root (DB path is relative).
- Verify: `go build ./... && go vet ./... && go test ./...`
- Smoke test: `curl localhost:8888/api/books` — book IDs contain colons (`spa-RVR1960:Gen`), so quote URLs. Path params are enum-validated: invalid book ID → 422.
- Health check: `GET /health` — 200 if the DB ping succeeds, 503 otherwise. The server shuts down gracefully on SIGTERM/SIGINT.

## Data & runtime gotchas
- `Bible.db` (~27MB) is the committed runtime datastore, opened by relative path in `main.go`. Never delete, regenerate, or rewrite it. `*.db-wal`/`*.db-shm` are gitignored runtime artifacts.
- Driver is `modernc.org/sqlite` (pure Go — no CGO required).
- Tables: `books`, `chapters`, `verses`. sqlx maps camelCase columns via `db:` tags. `text` and `order` are SQLite keywords and must be double-quoted in SQL (see existing queries).
- `/api/verses/search` matches against the precomputed accent-stripped column `verses.cleanTextAscii` (not `cleanText`); inputs go through `removeAccents()` then `escapeLike()` (LIKE wildcards are escaped via `ESCAPE '\'`).
- Book/chapter lookups match by prefix (`LIKE bookId || '.%'`), never `'%bookId%'` — IDs like `notspa-RVR1960:Gen.1` must not match `Gen`. `/api/books` builds its nested response from one `books LEFT JOIN chapters` query.
- Single-resource lookups (`db.Get`) return 404 when missing. List endpoints return `200 []`; range endpoints validate their boundary verses (404 if absent).
- Verse list endpoints accept opt-in `limit`/`offset` query params (`PaginationRequest`); omitted or `limit=0` returns the full result set, keeping the original contract. Huma rejects pointer params, so `0` is the "unset" sentinel — don't add `minimum:"1"` to them.

## Tests
- `main_test.go` seeds a throwaway SQLite DB in `t.TempDir()` and exercises endpoints through `newRouter(db)` (extracted from `main`) via `httptest` — tests never touch the committed `Bible.db`.
- Router construction reads `GO_ENV`; tests pin it with `t.Setenv("GO_ENV", "LOCAL")` so the PROD-only transformer path stays off.

## Environment
- `GO_ENV=LOCAL` for dev (see `.env.example`; loaded via godotenv). `GO_ENV=PROD|PRODUCTION` enables `CustomSchemaLinkTransformer` and OpenAPI server URLs, and requires `HOST_URL` — that code path is skipped entirely in local dev.

## Deployment
- Push to `master` triggers `.github/workflows/deploy-prod.yml`: `go vet` + `go test` run first; on success it cross-compiles `GOOS=linux GOARCH=arm64 go build .` and zips the whole repo (including `Bible.db`) to AWS Elastic Beanstalk. `.github/workflows/ci.yml` runs the same checks on pull requests.
- The `GO_VERSION` env in both workflows must match go.mod (currently 1.25.x); bump them together.
- `Procfile` runs `./spanish-bible-api-demo` — binary name comes from the module basename, so don't rename the module.
- `.platform/nginx/conf.d/cors.conf` is the EB nginx CORS config; keep it in sync with any CORS changes.
