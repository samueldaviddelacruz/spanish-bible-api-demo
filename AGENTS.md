# AGENTS.md

Single-package Go API (Huma v2 + chi + sqlx) serving Spanish Bible text from SQLite files — one per translation, currently RVR1960, LBLA and NVI. Everything lives in `main.go`; no sub-packages, no frontend.

## Commands
- Run: `go run .` — serves on `:8888` (override via `PORT` env). Must run from repo root (DB paths are relative).
- Verify: `go build ./... && go vet ./... && go test ./...`
- Smoke test: `curl localhost:8888/api/books` — book IDs contain colons (`spa-RVR1960:Gen`), so quote URLs. `?version=LBLA` reads another translation, and `curl localhost:8888/api/versions` lists them.
- A book ID is validated on its **shape** — a translation prefix in front of one of the 66 canon codes — so `spa-RVR1960:Genesis` is a 422 while `spa-RVR1960:Lev` is a 404. The distinction is deliberate and tested: the first is not a book ID, the second is a book this translation does not have. It used to be an enum of RVR1960's 66 ids, which cannot know that another translation's ids are just as valid.
- Health check: `GET /health` — 200 if **every** open translation can be read, 503 otherwise. The server shuts down gracefully on SIGTERM/SIGINT.

## Versions
One database per translation, all with the same schema, all committed:

| File | Ids | Reading |
| --- | --- | --- |
| `Bible.db` | `spa-RVR1960:...` | Reina-Valera 1960, the file the API grew up around |
| `Bible-LBLA.db` | `spa-LBLA:...` | La Biblia de las Américas |
| `Bible-NVI.db` | `spa-NVI:...` | Nueva Versión Internacional |

`openVersions` **discovers** them: `Bible.db` is RVR1960, and any `Bible-<NAME>.db` beside it is a translation named `<NAME>`. Discovery rather than configuration, and the same naming rule the CLI uses, so adding a translation is adding a file.

Every endpoint takes `?version=`; omitting it means RVR1960, and an unknown one is a 422 from the parameter's enum.

The enum is declared **once**, on `TranslationRequest`, which every input struct embeds — Huma reads embedded structs, so the parameter reaches all eight endpoints from one line. Adding a translation is therefore that one line plus a database file, and the values are the API's contract rather than something discovered at runtime: the three databases are committed and deployed together, so a spec listing all three is true in every deployment. A version listed whose file is absent is not a lie either — the store refuses it with a 422 naming the ones that are open.

`TestEveryEndpointDeclaresTheTranslations` asserts what that arrangement is for: every endpoint declares the parameter, and every one offers the same three values.

**The translation lives in the data, not in a column.** Every id carries it (`spa-LBLA:Gen.1.1`), which is why a second translation is a second file with no schema change and no query that knows the difference — a handler picks its database at the top and the SQL below it is untouched.

The two built translations come from the CLI, which owns the pipeline: `cmd/importjson` there turns a JSON export into one of these files, and its `importer_test.go` verifies every verse and heading against the export it came from. The exports are in neither repository — 7 MB each, and in copyright — so the databases are committed instead. To rebuild one, run the importer in the CLI repository and copy the file here.

## Data & runtime gotchas
- The database files are the committed runtime datastores, opened by relative path in `main.go`. Never delete, regenerate, or rewrite them; `Bible.db` in particular is the canonical one, byte-identical across the whole project. `*.db-wal`/`*.db-shm` are gitignored runtime artifacts. `Bible.db` is 26 MB and the two built files are 10 MB each — the shipped one carries free pages from when it was `ALTER TABLE`d, which is why the same content is smaller in the newer files.
- Driver is `modernc.org/sqlite` (pure Go — no CGO required).
- Tables: `books`, `chapters`, `verses`. sqlx maps camelCase columns via `db:` tags. `text` and `order` are SQLite keywords and must be double-quoted in SQL (see existing queries).
- `/api/verses/search` matches against the precomputed accent-stripped column `verses.cleanTextAscii` (not `cleanText`); inputs go through `removeAccents()` then `escapeLike()` (LIKE wildcards are escaped via `ESCAPE '\'`). The built translations fold that column in full where the shipped file folds it only partly; `LIKE` is case-insensitive either way, so search behaves the same against all three.
- Book/chapter lookups match by prefix (`LIKE bookId || '.%'`), never `'%bookId%'` — IDs like `notspa-RVR1960:Gen.1` must not match `Gen`. `/api/books` builds its nested response from one `books LEFT JOIN chapters` query.
- Single-resource lookups (`db.Get`) return 404 when missing. List endpoints return `200 []`; range endpoints validate their boundary verses (404 if absent).
- Verse list endpoints accept opt-in `limit`/`offset` query params (`PaginationRequest`); omitted or `limit=0` returns the full result set, keeping the original contract. Huma rejects pointer params, so `0` is the "unset" sentinel — don't add `minimum:"1"` to them.

## Tests
- `main_test.go` seeds a throwaway SQLite DB in `t.TempDir()` and exercises endpoints through `newRouter(oneVersion(db))` (both extracted from `main`) via `httptest` — tests never touch the committed databases.
- `TestSecondTranslationIsServed` is the one test that opens the committed files: it asserts `?version=LBLA` answers with `spa-LBLA:Gen` while no parameter still answers with `spa-RVR1960:Gen`, and it skips when the built databases are not there. `TestVersionsAreListed` and `TestUnknownVersionIsRejected` use the fixture.
- Router construction reads `GO_ENV`; tests pin it with `t.Setenv("GO_ENV", "LOCAL")` so the PROD-only transformer path stays off.
- `TestProdSchemaLink` covers the PROD path: `GO_ENV=PROD` + `HOST_URL` + `X-Forwarded-Host` → asserts the `$schema` URL in the response body.
- `TestProdOpenAPIServers` covers the `GO_ENV=PRODUCTION` arm: spec `servers` entries come from `HOST_URL`, and are absent in local dev. PROD refuses to start without `HOST_URL` (`log.Fatal`).

## Environment
- `GO_ENV=LOCAL` for dev (see `.env.example`; loaded via godotenv). `GO_ENV=PROD|PRODUCTION` sets the OpenAPI `servers` entries from `HOST_URL` (the API Gateway URL) and requires it — that code path is skipped entirely in local dev.
- The app sits behind AWS API Gateway (`/dev` stage path) in front of EB. `X-Forwarded-Host` is mapped at the gateway; huma's built-in `SchemaLinkTransformer` (installed by `huma.DefaultConfig`) uses it to build the `$schema` URLs in response bodies. Don't reintroduce a custom transformer fork — it was deleted on purpose.
- The transformer's `Link` header is silently dropped (humachi writes the status before transformers run); only the body `$schema` field reaches clients. Not a bug to chase.

## Deployment
- Push to `master` triggers `.github/workflows/deploy-prod.yml`: `go vet` + `go test` run first; on success it cross-compiles `GOOS=linux GOARCH=arm64 go build .` and zips the whole repo to AWS Elastic Beanstalk. That now means all three databases, so the bundle is about 45 MB — inside EB's limits, but it is why the deploy grew. `.github/workflows/ci.yml` runs the same checks on pull requests.
- The `GO_VERSION` env in both workflows must match go.mod (currently 1.25.x); bump them together.
- `Procfile` runs `./spanish-bible-api-demo` — binary name comes from the module basename, so don't rename the module.
- `.platform/nginx/conf.d/cors.conf` is the EB nginx CORS config; keep it in sync with any CORS changes.
