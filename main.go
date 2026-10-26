package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	_ "modernc.org/sqlite"
)

type Book struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Order     int       `json:"order"`
	Testament string    `json:"testament"`
	Chapters  []Chapter `json:"chapters"`
}

type bookChapterRow struct {
	ID        string         `db:"id"`
	Name      string         `db:"name"`
	Order     int            `db:"order"`
	Testament string         `db:"testament"`
	ChapterID sql.NullString `db:"chapterId"`
	Chapter   sql.NullInt64  `db:"chapter"`
	OsisEnd   sql.NullString `db:"osis_end"`
}
type Chapter struct {
	Chapter  int    `json:"chapter"`
	ID       string `json:"id"`
	Osis_End string `json:"osis_end"`
}
type Verse struct {
	ID            string `json:"id"`
	ChapterId     string `json:"chapterId" db:"chapterId"`
	CleanText     string `json:"cleanText" db:"cleanText"`
	Reference     string `json:"reference" db:"reference"`
	Text          string `json:"text" db:"text"`
	ChapterNumber int    `json:"chapterNumber" db:"chapterNumber"`
	VerseNumber   int    `json:"verseNumber" db:"verseNumber"`
}
type ListResponse[T any] struct {
	Body []T
}
type SingleResponse[T any] struct {
	Body T
}
type BookRequest struct {
	TranslationRequest
	BookId string `path:"bookId" doc:"Identificador del libro bíblico (ej: 'spa-RVR1960:Gen')"`
}

// PaginationRequest is opt-in: a request without `limit` gets every row, which is the API's
// original contract.
//
// Huma rejects pointer params — "pointers are not supported for form/header/path/query
// parameters", a panic at registration — so an absent `limit` reaches the handler as 0, and 0 is
// what paginateSQL reads as "no limit". minimum:"1" is what keeps that sentinel off the wire:
// Huma validates only the parameters a request actually carried, so an omitted `limit` stays
// legal while an explicit `limit=0` is a 422 instead of a silent request for the whole table.
type PaginationRequest struct {
	Limit  uint `query:"limit" minimum:"1" doc:"Número máximo de versículos a devolver (opcional; omitido = sin límite)"`
	Offset uint `query:"offset" doc:"Cantidad de versículos a omitir (opcional)"`
}

// VersesRequest is the API's one reading endpoint: a contiguous run of verses in one book.
//
// `from` and `to` use the notation readers already use — `1` is a chapter, `1:16` a verse — and both
// are optional. An omitted `from` starts at the beginning of the book; an omitted `to` ends at the
// end of the chapter `from` names. So `?from=1` is chapter 1, `?from=1:16` runs from 1:16 to the end
// of chapter 1, `?from=1&to=3` is three chapters, and no bounds at all is the whole book.
type VersesRequest struct {
	TranslationRequest
	BookRequest
	PaginationRequest
	From string `query:"from" pattern:"^[0-9]+(:[0-9]+)?$" example:"1:16" doc:"Inicio del rango: capítulo ('1') o capítulo:versículo ('1:16'). Omitido = principio del libro."`
	To   string `query:"to" pattern:"^[0-9]+(:[0-9]+)?$" example:"2:25" doc:"Fin del rango, en la misma notación. Omitido = final del capítulo indicado en 'from'."`
}

// VersesPathRequest is the shorthand form of the same request: the start of the range in the path.
//
// It exists because clients were already calling `/api/books/{bookId}/verses/1` and
// `…/verses/1:16` — 19,800 such requests in fifteen months of nginx logs, every one a 404 — so the
// endpoint meets the shape people had assumed rather than only the one it shipped with.
type VersesPathRequest struct {
	TranslationRequest
	BookRequest
	PaginationRequest
	From string `path:"from" required:"true" pattern:"^[0-9]+(:[0-9]+)?$" example:"1:16" doc:"Inicio del rango: capítulo ('1') o capítulo:versículo ('1:16')."`
	To   string `query:"to" pattern:"^[0-9]+(:[0-9]+)?$" example:"2:25" doc:"Fin del rango, en la misma notación. Omitido = final del capítulo indicado en 'from'."`
}

type SearchRequest struct {
	TranslationRequest
	Query  string `query:"q" required:"true" doc:"texto o termino a buscar"`
	Book   string `query:"book" doc:"Limita la búsqueda a un libro (ej: 'spa-RVR1960:Gen'). Omitido = toda la traducción."`
	Limit  uint   `query:"limit" default:"25" minimum:"1" maximum:"200" doc:"Máximo de resultados (por defecto 25, máximo 200)."`
	Offset uint   `query:"offset" doc:"Resultados a omitir (opcional)"`
}

// SearchResponse carries the total in a header rather than in the body: the body stays the same
// array every other list endpoint returns, and a client that wants the count reads one header
// instead of a different response shape.
type SearchResponse struct {
	Total int `header:"X-Total-Count" doc:"Total de versículos que coinciden, antes de limit y offset."`
	Body  []Verse
}

// reference is one end of a range: a chapter, and a verse within it when one was named.
type reference struct {
	chapter uint
	verse   uint // 0 means the whole chapter.
}

// bounds is a resolved, ordered range. wholeBook is the case with no bounds at all, where the query
// needs no positional predicate.
type bounds struct {
	fromChapter uint
	fromVerse   uint
	toChapter   uint
	toVerse     uint
	wholeBook   bool
}

// parseReference reads `1` or `1:16`. `pattern` has already rejected anything else, so this only has
// to parse — but it still rejects zero, which `pattern` cannot express.
func parseReference(name, value string) (reference, error) {
	if value == "" {
		return reference{}, nil
	}

	chapterText, verseText, hasVerse := strings.Cut(value, ":")
	chapter, err := strconv.ParseUint(chapterText, 10, 32)
	if err != nil || chapter == 0 {
		return reference{}, huma.Error422UnprocessableEntity("invalid range bound", &huma.ErrorDetail{
			Location: "query." + name,
			Message:  "expected a chapter number of at least 1",
			Value:    value,
		})
	}

	ref := reference{chapter: uint(chapter)}
	if !hasVerse {
		return ref, nil
	}

	verse, err := strconv.ParseUint(verseText, 10, 32)
	if err != nil || verse == 0 {
		return reference{}, huma.Error422UnprocessableEntity("invalid range bound", &huma.ErrorDetail{
			Location: "query." + name,
			Message:  "expected a verse number of at least 1 after the colon",
			Value:    value,
		})
	}
	ref.verse = uint(verse)
	return ref, nil
}

// resolveBounds turns the two query strings into an ordered range, applying the defaults that make
// the endpoint read naturally: no bounds is the whole book, and a missing `to` stops at the end of
// the chapter `from` names.
func resolveBounds(fromText, toText string) (bounds, error) {
	if fromText == "" && toText == "" {
		return bounds{wholeBook: true}, nil
	}

	from, err := parseReference("from", fromText)
	if err != nil {
		return bounds{}, err
	}
	to, err := parseReference("to", toText)
	if err != nil {
		return bounds{}, err
	}

	if to.chapter == 0 {
		to = reference{chapter: from.chapter}
	}
	if from.chapter == 0 {
		from = reference{chapter: 1}
	}

	if to.chapter < from.chapter ||
		(to.chapter == from.chapter && to.verse != 0 && to.verse < from.verse) {
		return bounds{}, huma.Error422UnprocessableEntity("the end of the range is before its start",
			&huma.ErrorDetail{
				Location: "query.to",
				Message:  "the end of the range is before its start",
				Value:    toText,
			})
	}

	return bounds{
		fromChapter: from.chapter,
		fromVerse:   from.verse,
		toChapter:   to.chapter,
		toVerse:     to.verse,
	}, nil
}

func Filter[T any](slice []T, f func(T) bool) []T {
	for i, value := range slice {
		if !f(value) {
			result := slices.Clone(slice[:i])
			for i++; i < len(slice); i++ {
				value = slice[i]
				if f(value) {
					result = append(result, value)
				}
			}
			return result
		}
	}
	return slice
}

func removeAccents(s string) string {
	// Create a Transformer chain:
	// 1. NFD (Normalization Form D): Decomposes characters into base characters and diacritics.
	// 2. runes.Remove(runes.In(unicode.Mn)): Removes all nonspacing marks (Mn category in Unicode).
	// 3. NFC (Normalization Form C): Recomposes characters where possible (optional, but good practice).
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

	// Apply the transformation to the string.
	output, _, err := transform.String(t, s)
	if err != nil {
		// Handle potential errors, e.g., print or return an empty string
		fmt.Printf("Error transforming string: %v\n", err)
		return s // Or handle error as appropriate for your application
	}
	return output
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	return strings.ReplaceAll(s, "_", `\_`)
}

func dbError(op string, err error) error {
	log.Printf("%s: %v", op, err)
	return huma.Error500InternalServerError("internal server error")
}

func paginateSQL(query string, args []any, p PaginationRequest) (string, []any) {
	if p.Limit == 0 {
		if p.Offset == 0 {
			return query, args
		}
		return query + " LIMIT -1 OFFSET ?", append(args, p.Offset)
	}
	return query + " LIMIT ? OFFSET ?", append(args, p.Limit, p.Offset)
}

// serveVerses is the body of both reading routes — the canonical `?from=&to=` form and the `{from}`
// path shorthand — so the two can never disagree about validation, ordering or paging.
func serveVerses(store versions, version, bookId, from, to string, page PaginationRequest) (*ListResponse[Verse], error) {
	db, versionErr := store.pick(version)
	if versionErr != nil {
		return nil, versionErr
	}
	if err := requireBook(bookId); err != nil {
		return nil, err
	}

	rangeBounds, err := resolveBounds(from, to)
	if err != nil {
		return nil, err
	}
	if err := requireBounds(db, bookId, rangeBounds); err != nil {
		return nil, err
	}

	query, args := versesQuery(bookId, rangeBounds, page)
	verses := []Verse{}
	if err := db.Select(&verses, query, args...); err != nil {
		return nil, dbError("error while getting verses from DB", err)
	}
	return &ListResponse[Verse]{Body: verses}, nil
}

// versesQuery builds the one reading query. The bounds are a row-value comparison on
// (chapterNumber, verseNumber), which orders in SQLite exactly as it orders on the page — so a run
// of verses is one scan, and limit/offset apply to it directly. The routes this replaced fetched
// every verse in the chapter range and then sliced the slice in Go, so `?limit=2` over fifty
// chapters still read all of them.
func versesQuery(bookId string, b bounds, p PaginationRequest) (string, []any) {
	query := `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber FROM verses WHERE chapterId LIKE ?`
	args := []any{bookId + ".%"}

	if !b.wholeBook {
		query += ` AND (chapterNumber, verseNumber) >= (?, ?)`
		args = append(args, b.fromChapter, b.fromVerse)

		toVerse := b.toVerse
		if toVerse == 0 {
			toVerse = math.MaxInt32 // an unnamed verse means the end of that chapter.
		}
		query += ` AND (chapterNumber, verseNumber) <= (?, ?)`
		args = append(args, b.toChapter, toVerse)
	}

	return paginateSQL(query+" ORDER BY chapterNumber, verseNumber", args, p)
}

// requireBounds answers 404 for a bound this translation does not have, rather than serving an empty
// list: a reader who asks for Genesis 1:99 has made a mistake worth reporting. Checking the start
// chapter too is what turns `?from=99` into an error instead of the silent `200 []` the old chapter
// route returned.
func requireBounds(db *sqlx.DB, bookId string, b bounds) error {
	if b.wholeBook {
		return nil
	}
	if err := requirePosition(db, bookId, b.fromChapter, b.fromVerse); err != nil {
		return err
	}
	return requirePosition(db, bookId, b.toChapter, b.toVerse)
}

// requirePosition checks one end of a range: a chapter when no verse was named, a verse when one was.
func requirePosition(db *sqlx.DB, bookId string, chapter, verse uint) error {
	var exists int

	if verse == 0 {
		err := db.Get(&exists, `SELECT 1 FROM chapters WHERE id = ?`, fmt.Sprintf("%s.%d", bookId, chapter))
		if errors.Is(err, sql.ErrNoRows) {
			return huma.Error404NotFound(fmt.Sprintf("chapter not found: %s.%d", bookId, chapter))
		}
		if err != nil {
			return dbError("error while checking chapter exists", err)
		}
		return nil
	}

	err := db.Get(&exists, `SELECT 1 FROM verses WHERE id = ?`, fmt.Sprintf("%s.%d.%d", bookId, chapter, verse))
	if errors.Is(err, sql.ErrNoRows) {
		return huma.Error404NotFound(fmt.Sprintf("verse not found: %s.%d.%d", bookId, chapter, verse))
	}
	if err != nil {
		return dbError("error while checking verse exists", err)
	}
	return nil
}

// DefaultVersion is the translation the shipped database holds, and what a request gets when it does
// not ask for one.
const DefaultVersion = "RVR1960"

// maxConnections bounds how many connections a translation may open.
//
// Reads come from one read-only file, so past the cores more of them buy nothing and cost a file
// handle each. The default is unbounded, which is a surprise under load rather than a limit under it.
const maxConnections = 8

// versions holds one open database per translation, all of them the same schema.
//
// A translation is a whole database rather than a column: the translation is carried in every id
// (`spa-LBLA:Gen.1.1`) and nothing else carries it, so a second translation is a second file with no
// schema change — and a request says which one it wants.
type versions struct {
	byName map[string]*sqlx.DB
	names  []string

	// fingerprint identifies the content these databases hold, and so versions every response for
	// caching. It comes from the files themselves rather than a build stamp, because the files are
	// what change.
	fingerprint string
}

// openVersions opens every translation in the working directory.
func openVersions() (versions, error) { return openVersionsIn(".") }

// openVersionsIn opens the databases in a directory: `Bible.db`, and any `Bible-<something>.db` beside
// it.
//
// The file names say where to **look**; the data says what each one **is**. A database names its own
// translation in every id (`spa-LBLA:Gen.1.1`) and in nothing else, which is precisely what lets a
// second translation be a second file — so the name of the file is no evidence of what is inside it.
// Trusting the name would serve a file called `Bible-copy.db` as "copy", and one renamed by mistake
// under the wrong translation entirely, quietly.
//
// The glob stays narrow so that a stray file in the directory — a backup, say — is not mistaken for a
// translation; anything it does match is opened and asked what it is.
func openVersionsIn(dir string) (versions, error) {
	found := versions{byName: map[string]*sqlx.DB{}}

	candidates := []string{filepath.Join(dir, "Bible.db")}
	others, err := filepath.Glob(filepath.Join(dir, "Bible-*.db"))
	if err != nil {
		return versions{}, err
	}
	candidates = append(candidates, others...)

	fingerprint := sha256.New()

	for _, path := range candidates {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		// Size and mtime identify the content without hashing 48 MB of pages at startup. The files
		// only change when the service is redeployed, which is exactly when a cached response should
		// stop being fresh.
		fmt.Fprintf(fingerprint, "%s:%d:%d;", path, info.Size(), info.ModTime().UnixNano())

		// The files are immutable: they arrive with the deployment and nothing here ever writes to
		// them. Saying so lets the driver skip locking and keeps SQLite from creating -wal/-shm
		// sidecars beside the data — and it is what lets the same binary serve from a read-only
		// filesystem, such as Lambda's /var/task.
		//
		// immutable=1 also means a leftover write-ahead log would be ignored rather than replayed, so
		// a sidecar is a reason to refuse to start rather than to serve stale text.
		if _, err := os.Stat(path + "-wal"); err == nil {
			return versions{}, fmt.Errorf("%s-wal exists: checkpoint the database before serving it", path)
		}
		db, err := sqlx.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
		if err != nil {
			return versions{}, fmt.Errorf("open %s: %w", path, err)
		}
		// The default pool opens a connection per request and keeps only two idle, which is churn for
		// a file that is opened read-only.
		db.SetMaxOpenConns(maxConnections)
		db.SetMaxIdleConns(maxConnections)
		// sqlx.Open is lazy, so a missing or unreadable file would otherwise fail at the first
		// request instead of here.
		if err := db.Ping(); err != nil {
			return versions{}, errors.Join(fmt.Errorf("open %s: %w", path, err), db.Close())
		}

		version, err := versionOf(db)
		if err != nil {
			return versions{}, errors.Join(fmt.Errorf("%s: %w", path, err), db.Close())
		}
		if _, taken := found.byName[version]; taken {
			return versions{}, errors.Join(
				fmt.Errorf("%s says it is %s, and so does another file", path, version),
				db.Close(),
			)
		}

		found.byName[version] = db
		found.names = append(found.names, version)
	}

	if len(found.names) == 0 {
		return versions{}, fmt.Errorf("no Bible database found in %s: expected Bible.db or Bible-<NAME>.db", dir)
	}
	found.fingerprint = hex.EncodeToString(fingerprint.Sum(nil))
	return found, nil
}

// versionOf reads the translation out of the database itself: `spa-LBLA:Gen` is LBLA.
func versionOf(db *sqlx.DB) (string, error) {
	const q = `SELECT id FROM books LIMIT 1`

	var id string
	if err := db.Get(&id, q); err != nil {
		return "", fmt.Errorf("read the version: %w", err)
	}

	prefix, _, ok := strings.Cut(id, ":")
	if !ok {
		return "", fmt.Errorf("book id %q does not name a version", id)
	}
	_, version, ok := strings.Cut(prefix, "-")
	if !ok || version == "" {
		return "", fmt.Errorf("book id %q does not name a version", id)
	}
	return version, nil
}

// TranslationRequest is the parameter every endpoint shares: which translation to read.
//
// Declared once and embedded, rather than repeated on each input, so adding a translation is one edit
// here and a file in the directory. The values are the API's contract — a client can rely on them —
// and a translation listed here whose database is missing is refused by the store with a 422 naming
// the ones that are open, rather than being a lie in the spec.
type TranslationRequest struct {
	Version string `query:"version" enum:"RVR1960,LBLA,NVI" doc:"Traducción a leer (por defecto, RVR1960)"`
}

// canonBooks is the 66 book codes, which every translation uses: the canon is the canon, and only the
// translation prefix in front of a code differs between them. The importer in the CLI keeps the same
// list honest against the shipped database.
var canonBooks = map[string]bool{
	"Gen": true, "Exod": true, "Lev": true, "Num": true, "Deut": true,
	"Josh": true, "Judg": true, "Ruth": true,
	"1Sam": true, "2Sam": true, "1Kgs": true, "2Kgs": true,
	"1Chr": true, "2Chr": true, "Ezra": true, "Neh": true, "Esth": true,
	"Job": true, "Ps": true, "Prov": true, "Eccl": true, "Song": true,
	"Isa": true, "Jer": true, "Lam": true, "Ezek": true, "Dan": true,
	"Hos": true, "Joel": true, "Amos": true, "Obad": true, "Jonah": true,
	"Mic": true, "Nah": true, "Hab": true, "Zeph": true, "Hag": true,
	"Zech": true, "Mal": true,
	"Matt": true, "Mark": true, "Luke": true, "John": true, "Acts": true,
	"Rom": true, "1Cor": true, "2Cor": true, "Gal": true, "Eph": true,
	"Phil": true, "Col": true, "1Thess": true, "2Thess": true,
	"1Tim": true, "2Tim": true, "Titus": true, "Phlm": true, "Heb": true,
	"Jas": true, "1Pet": true, "2Pet": true,
	"1John": true, "2John": true, "3John": true, "Jude": true, "Rev": true,
}

// requireBook rejects a request that does not name a book.
//
// The path parameter used to enforce this with an enum listing RVR1960's 66 ids, and an enum cannot
// know that another translation's ids are just as valid. So the check is on the *shape* of the id —
// a translation in front of one of the 66 codes — which keeps the two failures the API already
// distinguished: an id that is not a book (422), and a book this translation does not have (404, from
// the lookup that follows). Both matter, and neither is the other.
func requireBook(bookID string) error {
	prefix, code, ok := strings.Cut(bookID, ":")
	if !ok || prefix == "" || !canonBooks[code] {
		return huma.Error422UnprocessableEntity(fmt.Sprintf("not a book id: %q", bookID))
	}
	return nil
}

// ping reports whether every translation can be read, which is what being healthy means now that
// there is more than one of them.
func (v versions) ping() error {
	for _, name := range v.names {
		if err := v.byName[name].Ping(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// pick returns the database for a translation, or a 422 saying what is available: like a book id that
// does not exist, an unknown translation is the request's fault, not the server's.
func (v versions) pick(version string) (*sqlx.DB, error) {
	if version == "" {
		version = DefaultVersion
	}
	db, ok := v.byName[version]
	if !ok {
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("unknown version %q; available: %s", version, strings.Join(v.names, ", ")))
	}
	return db, nil
}

func main() {
	// Create a new router & API
	store, err := openVersions()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		for name, db := range store.byName {
			if err := db.Close(); err != nil {
				log.Printf("closing %s: %v", name, err)
			}
		}
	}()
	err = godotenv.Load()
	if err != nil {
		log.Println("No .env file found")
	}
	port := 8888
	if os.Getenv("PORT") != "" {
		port, err = strconv.Atoi(os.Getenv("PORT"))
		if err != nil {
			log.Fatal("Error while parsing port")
		}
	}

	router := newRouter(store)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		fmt.Printf("Starting server on port %d ", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}
}

// defaultSearchLimit is the page a search gets when the caller does not ask for one. Search is the
// only endpoint whose unbounded result is large — `q=Dios` matches 4,033 verses, 2 MB of JSON — so it
// pages by default where the reading endpoint deliberately does not.
const defaultSearchLimit = 25

// legacyRedirect points an old reading route at the endpoint that replaced it.
//
// The Location is deliberately **relative**. API Gateway strips its stage prefix (`/dev`) before the
// request reaches this process, so an absolute "/api/…" would send the client to the host root, out
// of the stage, and back into a 403. A relative reference is resolved by the client against its own
// URL — which still carries the prefix — so it works locally, behind the gateway and behind the
// tunnel without this service knowing anything about them.
func legacyRedirect(w http.ResponseWriter, r *http.Request, from string) {
	// "/api/books/{id}/verses/chapter/1/verse/2" → four segments follow "verses", so four "..".
	segments := strings.Split(r.URL.Path, "/")
	ups := 0
	for i, segment := range segments {
		if segment == "verses" {
			ups = len(segments) - i - 1
			break
		}
	}
	target := strings.Repeat("../", ups) + "verses?from=" + from

	// Header first, not http.Redirect: Redirect resolves a relative target against the request and
	// emits an absolute path, which is the very thing that breaks behind a stage prefix. Writing the
	// reference verbatim leaves the client to resolve it against its own URL.
	w.Header().Set("Location", withQuery(target, r))
	w.WriteHeader(http.StatusPermanentRedirect)
}

// withQuery carries the caller's query string (?version=, ?limit=, ?offset=) onto a redirect target
// that already carries a parameter of its own.
func withQuery(target string, r *http.Request) string {
	if r.URL.RawQuery == "" {
		return target
	}
	return target + "&" + r.URL.RawQuery
}

// cacheable marks responses as revalidatable. Every response is a pure function of the databases,
// and their fingerprint changes only when the service is redeployed, so the fingerprint is a valid
// version for every URL — including `/openapi.json` and the schema documents.
func cacheable(fingerprint string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path == "/health" {
			next.ServeHTTP(w, r) // a health check is a live probe, not content worth caching.
			return
		}

		tag := etagFor(fingerprint, r.URL.RequestURI())
		if etagMatches(r.Header.Get("If-None-Match"), tag) {
			w.Header().Set("ETag", tag)
			w.Header().Set("Cache-Control", "public, max-age=3600")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		next.ServeHTTP(&cacheOn200{ResponseWriter: w, tag: tag}, r)
	})
}

// cacheOn200 adds the cache metadata once the handler has actually produced a 200.
//
// Marking the response before the handler runs would be simpler, but it advertises a 404 or a 422 as
// cacheable too — and now that a CDN is in front of this service, an hour-long TTL on an error is a
// real problem: the bot traffic that walks mistyped paths would fill the edge cache with junk, and a
// caller who mistypes a range would keep seeing that error after fixing it.
type cacheOn200 struct {
	http.ResponseWriter
	tag       string
	wroteHead bool
}

func (c *cacheOn200) WriteHeader(status int) {
	if !c.wroteHead {
		c.wroteHead = true
		if status == http.StatusOK {
			c.Header().Set("ETag", c.tag)
			c.Header().Set("Cache-Control", "public, max-age=3600")
		} else {
			c.Header().Set("Cache-Control", "no-store")
		}
	}
	c.ResponseWriter.WriteHeader(status)
}

func (c *cacheOn200) Write(b []byte) (int, error) {
	if !c.wroteHead {
		c.WriteHeader(http.StatusOK) // net/http implies 200 on the first write; do it explicitly.
	}
	return c.ResponseWriter.Write(b)
}

// etagFor derives a tag from the request and the data fingerprint rather than from the response
// body: hashing bodies would mean buffering them, and the largest legal response is megabytes.
func etagFor(fingerprint, uri string) string {
	sum := sha256.Sum256([]byte(fingerprint + " " + uri))
	return `W/"` + hex.EncodeToString(sum[:8]) + `"`
}

// etagMatches is the If-None-Match comparison: a comma-separated list in which either side may be
// the wildcard.
func etagMatches(header, tag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		if candidate = strings.TrimSpace(candidate); candidate == "*" || candidate == tag {
			return true
		}
	}
	return false
}

// docsHTML is the API reference page, served by us rather than by Huma's built-in handler for one
// reason: Huma writes `apiDescriptionUrl="/openapi.yaml"` with a leading slash. A page served from
// `/dev/docs` then fetches `/openapi.yaml` at the origin root — outside the stage prefix, which is a
// 403 — and the reference renders blank. A relative reference is resolved against the page's own
// URL, so "openapi.yaml" becomes `/dev/openapi.yaml` behind the gateway and `/openapi.yaml` on the
// tunnel hostname, without this service knowing which it is behind.
const docsHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="referrer" content="no-referrer">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>API Biblia en español Reference</title>
    <link rel="stylesheet" href="https://unpkg.com/@stoplight/elements@9.0.15/styles.min.css" crossorigin integrity="sha384-iVQBHadsD+eV0M5+ubRCEVXrXEBj+BqcuwjUwPoVJc0Pb1fmrhYSAhL+BFProHdV">
    <script src="https://unpkg.com/@stoplight/elements@9.0.15/web-components.min.js" crossorigin integrity="sha384-xjOcq9PZ/k+pGtPS/xcsCRXGjKKfTlIa4H1IYEnC+97jNa6sAMWTNrV6hY08W3GL"></script>
  </head>
  <body style="height: 100vh;">
    <elements-api
      apiDescriptionUrl="openapi.yaml"
      router="hash"
      layout="sidebar"
      tryItCredentialsPolicy="same-origin"
    ></elements-api>
  </body>
</html>
`

func newRouter(store versions) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if err := store.ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, docsHTML)
	})

	// The two reading routes that carried real traffic before the merge: 4,978 and 594 requests in
	// the last fifteen months of nginx logs. They are 308s rather than handlers — one line each, and
	// their behaviour cannot drift from the endpoint they point at. The three range variants got 351
	// requests between them, one of which was a single request, and are simply gone.
	mux.HandleFunc("GET /api/books/{bookId}/verses/chapter/{chapterNumber}", func(w http.ResponseWriter, r *http.Request) {
		legacyRedirect(w, r, r.PathValue("chapterNumber"))
	})
	mux.HandleFunc("GET /api/books/{bookId}/verses/chapter/{chapterNumber}/verse/{verseNumber}", func(w http.ResponseWriter, r *http.Request) {
		legacyRedirect(w, r, r.PathValue("chapterNumber")+":"+r.PathValue("verseNumber"))
	})

	config := huma.DefaultConfig("API Biblia en español", "1.0.0")
	// The built-in docs page is replaced below: it hardcodes an absolute spec URL, which breaks
	// behind a path prefix. See docsHTML.
	config.DocsPath = ""
	config.Info.Contact = &huma.Contact{
		Name:  "Samuel De La Cruz",
		Email: "delacruzportorrealsamueldavid@gmail.com",
	}
	config.Info.Description = `## 📘 Descripción de la API

Esta API proporciona acceso estructurado al texto bíblico en varias traducciones al español. Permite consultar libros, capítulos y versículos específicos de la Biblia, facilitando la navegación por las Escrituras de manera programática. Está pensada para ser utilizada por aplicaciones web, móviles o sistemas que necesiten integrar o mostrar contenido bíblico de forma precisa y eficiente.

---

### ✨ Funcionalidades principales

- Obtener la lista completa de libros bíblicos (Antiguo y Nuevo Testamento).
- Consultar un libro específico por su ID.
- Listar todos los capítulos o versículos de un libro o capítulo determinado.
- Buscar un rango de versículos entre capítulos o dentro de un capítulo.
- Acceso a versículos individuales mediante referencias precisas.

---

### 🏷️ Formato y estructura

- Todos los recursos están organizados por identificadores únicos consistentes (libro.capítulo.versículo).
- Las respuestas están optimizadas para lecturas rápidas y ordenadas por capítulo y versículo.

---

### 🔒 Notas

Sirve las traducciones que tenga abiertas — **Reina-Valera 1960** (por defecto), **LBLA** y **NVI** — y
cada endpoint acepta el parámetro version= para elegir una; GET /api/versions dice cuáles hay.

No contiene comentarios ni notas teológicas.
`
	env := os.Getenv("GO_ENV")

	if env == "PROD" || env == "PRODUCTION" {
		hostUrl := os.Getenv("HOST_URL")
		if hostUrl == "" {
			log.Fatal("HOST_URL must be set when GO_ENV is PROD")
		}
		hostPath := "dev"
		serverUrl := fmt.Sprintf("%s/%s", hostUrl, hostPath)
		config.Servers = []*huma.Server{
			{
				URL:         serverUrl,
				Description: "API URL",
			},
		}
	}
	api := humago.New(mux, config)

	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/books",
		Summary:     "Obtener todos los libros de la Biblia",
		Description: "Devuelve la lista completa de libros de la traducción elegida, incluyendo información del testamento y los capítulos correspondientes.",
		Tags:        []string{"Books"},
	}, func(ctx context.Context, i *TranslationRequest) (*ListResponse[Book], error) {
		db, versionErr := store.pick(i.Version)
		if versionErr != nil {
			return nil, versionErr
		}
		rows := []bookChapterRow{}
		err := db.Select(&rows, `SELECT b.id, b.name, b."order", b.testament, c.id AS chapterId, c.chapter, c.osis_end
								FROM books b
								LEFT JOIN chapters c ON c.id LIKE b.id || '.%'
								ORDER BY b."order", c.chapter`)
		if err != nil {
			return nil, dbError("error while getting books from DB", err)
		}

		books := []Book{}
		index := map[string]int{}
		for _, row := range rows {
			pos, ok := index[row.ID]
			if !ok {
				books = append(books, Book{ID: row.ID, Name: row.Name, Order: row.Order, Testament: row.Testament, Chapters: []Chapter{}})
				pos = len(books) - 1
				index[row.ID] = pos
			}
			if row.ChapterID.Valid {
				books[pos].Chapters = append(books[pos].Chapters, Chapter{Chapter: int(row.Chapter.Int64), ID: row.ChapterID.String, Osis_End: row.OsisEnd.String})
			}
		}
		return &ListResponse[Book]{
			Body: books,
		}, nil
	})

	huma.Register(api, huma.Operation{
		Method: http.MethodGet,

		Path:        "/api/books/{bookId}",
		Summary:     "Obtener un libro específico",
		Description: "Devuelve los detalles de un libro de la traducción elegida a partir de su ID, que lleva la traducción (por ejemplo, spa-LBLA:Gen), incluyendo los capítulos que lo componen.",
		Tags:        []string{"Book"},
	}, func(ctx context.Context, input *BookRequest) (*SingleResponse[Book], error) {
		db, versionErr := store.pick(input.Version)
		if versionErr != nil {
			return nil, versionErr
		}
		if err := requireBook(input.BookId); err != nil {
			return nil, err
		}
		book := Book{}

		err := db.Get(&book, `SELECT id, name, "order", testament FROM books WHERE id = ?`, input.BookId)
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, dbError("error while getting book from DB", err)
			}
			return nil, huma.Error404NotFound(fmt.Sprintf("Book not found: %s", input.BookId))
		}
		err = db.Select(&book.Chapters, "SELECT * FROM chapters WHERE id like ? ORDER BY chapter", book.ID+".%")
		if err != nil {
			return nil, dbError("error while getting chapters from DB", err)
		}

		return &SingleResponse[Book]{
			Body: book,
		}, nil
	})

	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/books/{bookId}/verses",
		Summary:     "Obtener versículos de un libro",
		Description: "Devuelve un tramo continuo de versículos. Sin parámetros devuelve el libro entero; `from` y `to` aceptan `capítulo` o `capítulo:versículo`, así que `?from=1` es el capítulo 1, `?from=1:16` va de 1:16 al final del capítulo 1, `?from=1&to=3` son los capítulos 1 a 3 y `?from=1:1&to=2:25` es un rango entre versículos de dos capítulos.",
		Tags:        []string{"Verses"},
	}, func(ctx context.Context, input *VersesRequest) (*ListResponse[Verse], error) {
		return serveVerses(store, input.Version, input.BookId, input.From, input.To, input.PaginationRequest)
	})

	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/books/{bookId}/verses/{from}",
		Summary:     "Obtener versículos desde un capítulo o un versículo",
		Description: "Forma abreviada de `/api/books/{bookId}/verses`, con el inicio del rango en la ruta: `/verses/1` es el capítulo 1, `/verses/1:16` va de 1:16 al final del capítulo 1, y `?to=` cierra el rango (`?to=3`, `?to=2:25`). Devuelve lo mismo que la forma con parámetros.",
		Tags:        []string{"Verses"},
	}, func(ctx context.Context, input *VersesPathRequest) (*ListResponse[Verse], error) {
		return serveVerses(store, input.Version, input.BookId, input.From, input.To, input.PaginationRequest)
	})

	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/versions",
		Summary:     "Obtener las traducciones disponibles",
		Description: "Devuelve las traducciones que esta instancia sirve. Cada endpoint admite `?version=`; sin él se usa " + DefaultVersion + ".",
		Tags:        []string{"Versions"},
	}, func(ctx context.Context, i *struct{}) (*ListResponse[string], error) {
		return &ListResponse[string]{Body: store.names}, nil
	})

	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/verses/search",
		Summary:     "Buscar dentro de los versiculos de la biblia",
		Description: "Devuelve los versículos que contengan el texto especificado. `limit` (por defecto 25, máximo 200) y `offset` recortan `body`, así que el total de coincidencias viaja en la cabecera `X-Total-Count`. `?book=` limita la búsqueda a un libro.",
		Tags:        []string{"Verses"},
	}, func(ctx context.Context, input *SearchRequest) (*SearchResponse, error) {
		db, versionErr := store.pick(input.Version)
		if versionErr != nil {
			return nil, versionErr
		}
		if input.Book != "" {
			if err := requireBook(input.Book); err != nil {
				return nil, err
			}
		}

		// One predicate feeds both the count and the page, so the header cannot describe a
		// different query than the body did.
		where := `WHERE cleanTextAscii LIKE ? ESCAPE '\'`
		args := []any{"%" + escapeLike(removeAccents(input.Query)) + "%"}
		if input.Book != "" {
			where += ` AND chapterId LIKE ? ESCAPE '\'`
			args = append(args, escapeLike(input.Book)+".%")
		}

		var total int
		if err := db.Get(&total, `SELECT count(*) FROM verses `+where, args...); err != nil {
			return nil, dbError("error while counting search results", err)
		}

		limit := input.Limit
		if limit == 0 {
			limit = defaultSearchLimit
		}
		query := `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber FROM verses ` +
			where + ` ORDER BY chapterNumber, verseNumber LIMIT ? OFFSET ?`

		verses := []Verse{}
		if err := db.Select(&verses, query, append(args, limit, input.Offset)...); err != nil {
			return nil, dbError("error while searching verses in DB", err)
		}

		return &SearchResponse{
			Total: total,
			Body:  verses,
		}, nil
	})

	return cacheable(store.fingerprint, mux)
}
