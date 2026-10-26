# AGENTS.md

Single-package Go API (Huma v2 on `net/http`'s `ServeMux` via the `humago` adapter, plus sqlx) serving Spanish Bible text from SQLite files — one per translation, currently RVR1960, LBLA and NVI. Everything lives in `main.go`; no sub-packages, no frontend.

## Endpoints

Six operations, all `GET` (plus two redirects that exist only for old clients):

| Endpoint | What it answers |
| --- | --- |
| `/api/versions` | which translations this instance serves |
| `/api/books` | every book with its chapters — the navigation index |
| `/api/books/{bookId}` | one book with its chapters |
| `/api/books/{bookId}/verses` | a run of verses: a chapter, a range, or a whole book |
| `/api/books/{bookId}/verses/{from}` | the same, with the start of the range in the path |
| `/api/verses/search` | full-text search, optionally within one book |

`/api/books/{bookId}/verses` is deliberately the only reading endpoint. It replaced five routes — a chapter, a single verse, a chapter range, a chapter-to-verse range, and a verse range — that were one query with different bounds. Bounds use the notation readers already use: `1` is a chapter, `1:16` is a verse, and both parameters are optional.

- no bounds → the whole book
- `?from=1` → chapter 1, because an omitted `to` ends at the end of the chapter `from` names
- `?from=1:16` → 1:16 to the end of chapter 1
- `?from=1&to=3` → chapters 1 to 3
- `?from=1:1&to=2:25` → across a chapter boundary

**The `/{from}` form is not redundant, so don't delete it.** Fifteen months of nginx access logs showed ~19,800 requests to `/api/books/{book}/verses/1` and `…/verses/1:16` — a Flutter client (user agents `Dart/3.x`) that assumed exactly this shape and got a 404 every time, from August 2025 into October 2026. It feeds the same `serveVerses` as the query form, differs only in where the bound is written, and `?to=` closes the range.

A bound that does not exist is a **404**; an inverted or unparsable range is a **422**; an omitted bound is never an error. There is no single-verse route: ask for the run, or `?from=1:1&to=1:1`.

**Two old routes survive as 308s**, in `newRouter` rather than as Huma operations: `…/verses/chapter/{n}` and `…/verses/chapter/{n}/verse/{v}`, which carried 4,978 and 594 requests in those same logs. They redirect to the new endpoint with the caller's query string (`?version=`, `?limit=`) preserved. The three range variants got 351 requests between them — one of them exactly once — and are gone outright. If a request arrives on a redirect, its `Location` is the canonical URL, so nothing downstream needs to know the old shape.

## Commands
- Run: `go run .` — serves on `:8888` (override via `PORT` env). Must run from repo root (DB paths are relative).
- Verify: `go build ./... && go vet ./... && go test ./...`
- Smoke test: `curl localhost:8888/api/books` — book IDs contain colons (`spa-RVR1960:Gen`), so quote URLs. `?version=LBLA` reads another translation, and `curl localhost:8888/api/versions` lists them.
- A book ID is validated on its **shape** — a translation prefix in front of one of the 66 canon codes — so `spa-RVR1960:Genesis` is a 422 (no such canon code) while `spa-RVR1960:Gen` is a 200. A code that is well-formed but absent from that translation is a 404 instead. The distinction is deliberate and tested — `TestGetBook` asks for `spa-RVR1960:Rev` against the fixture, which seeds only a few books — though it is unreachable against the shipped files, where all three translations carry all 66. It used to be an enum of RVR1960's 66 ids, which cannot know that another translation's ids are just as valid.
- Health check: `GET /health` — 200 if **every** open translation can be read, 503 otherwise. The server shuts down gracefully on SIGTERM/SIGINT.

## Versions
One database per translation, all with the same schema, all committed:

| File | Ids | Reading |
| --- | --- | --- |
| `Bible.db` | `spa-RVR1960:...` | Reina-Valera 1960, the file the API grew up around |
| `Bible-LBLA.db` | `spa-LBLA:...` | La Biblia de las Américas |
| `Bible-NVI.db` | `spa-NVI:...` | Nueva Versión Internacional |

`openVersions` **discovers** them: `Bible.db` is RVR1960, and any `Bible-<NAME>.db` beside it is a translation named `<NAME>`. Discovery rather than configuration, and the same naming rule the CLI uses, so adding a translation is adding a file.

Every endpoint except `/api/versions` takes `?version=`; omitting it means RVR1960, and an unknown one is a 422 from the parameter's enum.

The enum is declared **once**, on `TranslationRequest`, which every input struct embeds — Huma reads embedded structs, so the parameter reaches every endpoint that takes one, from a single line. Adding a translation is therefore that one line plus a database file, and the values are the API's contract rather than something discovered at runtime: the three databases are committed and deployed together, so a spec listing all three is true in every deployment. A version listed whose file is absent is not a lie either — the store refuses it with a 422 naming the ones that are open.

`TestEveryEndpointDeclaresTheTranslations` asserts what that arrangement is for: every endpoint declares the parameter, and every one offers the same three values.

**The translation lives in the data, not in a column.** Every id carries it (`spa-LBLA:Gen.1.1`), which is why a second translation is a second file with no schema change and no query that knows the difference — a handler picks its database at the top and the SQL below it is untouched.

The two built translations come from the CLI, which owns the pipeline: `cmd/importjson` there turns a JSON export into one of these files, and its `importer_test.go` verifies every verse and heading against the export it came from. The exports are in neither repository — 7 MB each, and in copyright — so the databases are committed instead. To rebuild one, run the importer in the CLI repository and copy the file here.

## Data & runtime gotchas
- The database files are the committed runtime datastores, opened by relative path in `main.go`. Never delete, regenerate, or rewrite them; `Bible.db` in particular is the canonical one, byte-identical across the whole project. `*.db-wal`/`*.db-shm` are gitignored runtime artifacts. `Bible.db` is 26 MB and the two built files are 10 MB each — the shipped one carries free pages from when it was `ALTER TABLE`d, which is why the same content is smaller in the newer files.
- Driver is `modernc.org/sqlite` (pure Go — no CGO required).
- Tables: `books`, `chapters`, `verses`. sqlx maps camelCase columns via `db:` tags. `text` and `order` are SQLite keywords and must be double-quoted in SQL (see existing queries).
- `/api/verses/search` matches against the precomputed accent-stripped column `verses.cleanTextAscii` (not `cleanText`); inputs go through `removeAccents()` then `escapeLike()` (LIKE wildcards are escaped via `ESCAPE '\'`). The built translations fold that column in full where the shipped file folds it only partly; `LIKE` is case-insensitive either way, so search behaves the same against all three.
- Book/chapter lookups match by prefix (`LIKE bookId || '.%'`), never `'%bookId%'` — IDs like `notspa-RVR1960:Gen.1` must not match `Gen`. `/api/books` builds its nested response from one `books LEFT JOIN chapters` query.
- A missing thing is a **404**, not an empty list: `/api/books/{bookId}` when the book is absent, and `/api/books/{bookId}/verses` when a **named** bound does not exist (the start chapter is checked too, which is why `?from=99` is an error rather than the `200 []` the old chapter route returned). A range with no bounds is the whole book and always has verses, so `200 []` now only means a filter genuinely matched nothing — that is search.
- The reading endpoint accepts opt-in `limit`/`offset` query params (`PaginationRequest`); an omitted `limit` returns the whole range, which is the original contract. Huma rejects pointer params — it panics at registration with `pointers are not supported for form/header/path/query parameters` — so an absent `limit` reaches the handler as `0`, and `0` is what `paginateSQL` reads as "no limit". `minimum:"1"` keeps that sentinel off the wire: Huma validates **only the parameters a request actually carried**, so an omitted `limit` stays legal while an explicit `limit=0` is a 422. Verified against huma v2.37.3 — don't drop the tag, and don't reintroduce a `limit=0` contract in the docs or tests.
- Search is the exception to that contract: it pages **by default** (`default:"25"`, `maximum:"200"`), because its unbounded result set is the one that is large — `q=Dios` matches 4,033 verses, 2 MB of JSON. The match count comes back in `X-Total-Count` rather than in the body, so the body stays the same array shape as every other list endpoint while `limit`/`offset` trim it. `?book=` narrows the search to one book.
- Responses carry `ETag` and `Cache-Control: public, max-age=3600`, both keyed to a fingerprint of the open database files (path+size+mtime, computed in `openVersionsIn`). A redeploy that ships different databases therefore invalidates every cached URL; `/health` is excluded because it is a live probe rather than content. The tag is derived from the fingerprint plus the request URI, never from the response body — hashing bodies would mean buffering megabyte responses.

## Tests
- `main_test.go` seeds a throwaway SQLite DB in `t.TempDir()` and exercises endpoints through `newRouter(oneVersion(db))` (both extracted from `main`) via `httptest` — tests never touch the committed databases.
- `TestSecondTranslationIsServed` is the one test that opens the committed files: it asserts `?version=LBLA` answers with `spa-LBLA:Gen` while no parameter still answers with `spa-RVR1960:Gen`, and it skips when the built databases are not there. `TestVersionsAreListed` and `TestUnknownVersionIsRejected` use the fixture.
- Router construction reads `GO_ENV`; tests pin it with `t.Setenv("GO_ENV", "LOCAL")` so the PROD-only transformer path stays off.
- `TestProdSchemaLink` covers the PROD path: `GO_ENV=PROD` + `HOST_URL` + `X-Forwarded-Host` → asserts the `$schema` URL in the response body.
- `TestProdOpenAPIServers` covers the `GO_ENV=PRODUCTION` arm: spec `servers` entries come from `HOST_URL`, and are absent in local dev. PROD refuses to start without `HOST_URL` (`log.Fatal`).

## Environment
- `GO_ENV=LOCAL` for dev (see `.env.example`; loaded via godotenv). `GO_ENV=PROD|PRODUCTION` sets the OpenAPI `servers` entries from `HOST_URL` (the API Gateway URL) and requires it — that code path is skipped entirely in local dev.
- The app sits behind AWS API Gateway (`/dev` stage path) in front of EB. `X-Forwarded-Host` is mapped at the gateway; huma's built-in `SchemaLinkTransformer` (installed by `huma.DefaultConfig`) uses it to build the `$schema` URLs in response bodies. Don't reintroduce a custom transformer fork — it was deleted on purpose.
- The transformer's `Link` header is silently dropped — a property of Huma's own response path rather than of the router adapter (checked against both `humachi` and `humago`); only the body `$schema` field reaches clients. Not a bug to chase.

## Deployment
- Push to `master` triggers `.github/workflows/deploy-prod.yml`: `gofmt -l`, `go vet` and `go test` run first; on success it cross-compiles `GOOS=linux GOARCH=arm64 go build .` and zips the whole repo to AWS Elastic Beanstalk. That now means all three databases, so the bundle is about 62 MB raw and about 19 MB zipped — inside EB's limits, but it is why the deploy grew (it is also what lets the same zip fit Lambda's 50 MB direct-upload limit, if that ever matters). `.github/workflows/ci.yml` runs the same checks on pull requests.
- Both workflows read the Go version from `go.mod` (`go-version-file`), so it cannot drift from the toolchain the module declares.
- `Procfile` runs `./spanish-bible-api-demo` — binary name comes from the module basename, so don't rename the module.
- `.platform/nginx/conf.d/cors.conf` is the EB nginx CORS config; keep it in sync with any CORS changes.
