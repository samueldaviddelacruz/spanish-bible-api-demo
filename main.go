package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
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
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
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
	// Version selects the translation; empty means the shipped one.
	Version string `query:"version" doc:"Traducción a leer: RVR1960 (por defecto), LBLA o NVI"`
	BookId  string `path:"bookId" doc:"Identificador del libro bíblico (ej: 'spa-RVR1960:Gen')"`
}

type PaginationRequest struct {
	Limit  uint `query:"limit" doc:"Número máximo de versículos a devolver (opcional, 0 u omitido = sin límite)"`
	Offset uint `query:"offset" doc:"Cantidad de versículos a omitir (opcional)"`
}

type VersesByChapterIdRequest struct {
	// Version selects the translation; empty means the shipped one.
	Version string `query:"version" doc:"Traducción a leer: RVR1960 (por defecto), LBLA o NVI"`
	BookRequest
	PaginationRequest
	ChapterNumber uint `path:"chapterNumber" required:"true" doc:"Número del capítulo del cual obtener los versículos"`
}

type VerseRequest struct {
	// Version selects the translation; empty means the shipped one.
	Version string `query:"version" doc:"Traducción a leer: RVR1960 (por defecto), LBLA o NVI"`
	BookRequest
	ChapterNumber uint `path:"chapterNumber" required:"true" doc:"Número del capítulo que contiene el versículo"`
	VerseNumber   uint `path:"verseNumber" required:"true" doc:"Número del versículo a obtener"`
}

type SearchRequest struct {
	// Version selects the translation; empty means the shipped one.
	Version string `query:"version" doc:"Traducción a leer: RVR1960 (por defecto), LBLA o NVI"`
	Query   string `query:"q" required:"true" doc:"texto o termino a buscar"`
	PaginationRequest
}

type ChapterToChapterVersesRequest struct {
	// Version selects the translation; empty means the shipped one.
	Version string `query:"version" doc:"Traducción a leer: RVR1960 (por defecto), LBLA o NVI"`
	BookRequest
	PaginationRequest
	StartChapterNumber uint `path:"startChapterNumber" required:"true" doc:"Capítulo inicial del rango"`
	EndChapterNumber   uint `path:"endChapterNumber" required:"true" doc:"Capítulo final del rango"`
	EndVerseNumber     uint `path:"endVerseNumber" required:"true" doc:"Último versículo a incluir del capítulo final"`
}

type VerseRangeRequest struct {
	// Version selects the translation; empty means the shipped one.
	Version string `query:"version" doc:"Traducción a leer: RVR1960 (por defecto), LBLA o NVI"`
	BookRequest
	PaginationRequest
	StartChapterNumber uint `path:"startChapterNumber" required:"true" doc:"Capítulo inicial"`
	StartVerseNumber   uint `path:"startVerseNumber" required:"true" doc:"Versículo inicial dentro del capítulo inicial"`
	EndChapterNumber   uint `path:"endChapterNumber" required:"true" doc:"Capítulo final"`
	EndVerseNumber     uint `path:"endVerseNumber" required:"true" doc:"Versículo final dentro del capítulo final"`
}

type ChapterRangeRequest struct {
	// Version selects the translation; empty means the shipped one.
	Version string `query:"version" doc:"Traducción a leer: RVR1960 (por defecto), LBLA o NVI"`
	BookRequest
	PaginationRequest
	StartChapterNumber uint `path:"startChapterNumber" required:"true" doc:"Capítulo inicial"`
	EndChapterNumber   uint `path:"endChapterNumber" required:"true" doc:"Capítulo final"`
}

func (i *ChapterToChapterVersesRequest) Resolve(ctx huma.Context) []error {
	if i.EndChapterNumber < i.StartChapterNumber {
		return []error{&huma.ErrorDetail{
			Location: "path.endChapterNumber",
			Message:  "endChapterNumber cannot be less than startChapterNumber",
			Value:    i.StartChapterNumber,
		}}
	}
	return nil
}
func (i *VerseRangeRequest) Resolve(ctx huma.Context) []error {
	if i.EndChapterNumber < i.StartChapterNumber {
		return []error{&huma.ErrorDetail{
			Location: "path.endChapterNumber",
			Message:  "endChapterNumber cannot be less than startChapterNumber",
			Value:    i.StartChapterNumber,
		}}
	}
	return nil
}
func (i *ChapterRangeRequest) Resolve(ctx huma.Context) []error {
	if i.EndChapterNumber < i.StartChapterNumber {
		return []error{&huma.ErrorDetail{
			Location: "path.endChapterNumber",
			Message:  "endChapterNumber cannot be less than startChapterNumber",
			Value:    i.StartChapterNumber,
		}}
	}
	return nil
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

func paginate[T any](items []T, p PaginationRequest) []T {
	if p.Limit == 0 && p.Offset == 0 {
		return items
	}
	if p.Offset >= uint(len(items)) {
		return []T{}
	}
	items = items[p.Offset:]
	if p.Limit != 0 && uint(len(items)) > p.Limit {
		items = items[:p.Limit]
	}
	return items
}

// DefaultVersion is the translation the shipped database holds, and what a request gets when it does
// not ask for one.
const DefaultVersion = "RVR1960"

// versions holds one open database per translation, all of them the same schema.
//
// A translation is a whole database rather than a column: the translation is carried in every id
// (`spa-LBLA:Gen.1.1`) and nothing else carries it, so a second translation is a second file with no
// schema change — and a request says which one it wants.
type versions struct {
	byName map[string]*sqlx.DB
	names  []string
}

// openVersions opens every translation in the working directory: the shipped `Bible.db`, which is
// RVR1960, and any `Bible-<NAME>.db` beside it. Discovery rather than configuration, and the same
// naming rule the CLI uses, so adding a translation means adding a file.
func openVersions() (versions, error) {
	found := versions{byName: map[string]*sqlx.DB{}}

	add := func(name, path string) error {
		db, err := sqlx.Open("sqlite", path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		// sqlx.Open is lazy, so a missing or unreadable file would otherwise fail at the first
		// request instead of here.
		if err := db.Ping(); err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		found.byName[name] = db
		found.names = append(found.names, name)
		return nil
	}

	if _, err := os.Stat("Bible.db"); err == nil {
		if err := add(DefaultVersion, "Bible.db"); err != nil {
			return versions{}, err
		}
	}

	others, err := filepath.Glob("Bible-*.db")
	if err != nil {
		return versions{}, err
	}
	for _, path := range others {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "Bible-"), ".db")
		if name == "" {
			continue
		}
		if err := add(name, path); err != nil {
			return versions{}, err
		}
	}

	if len(found.names) == 0 {
		return versions{}, fmt.Errorf("no Bible database found: expected Bible.db or Bible-<NAME>.db")
	}
	return found, nil
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
		for _, db := range store.byName {
			db.Close()
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
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

func newRouter(store versions) *chi.Mux {
	router := chi.NewMux()

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := store.ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	config := huma.DefaultConfig("RV 1960 API", "1.0.0")
	config.Info.Contact = &huma.Contact{
		Name:  "Samuel De La Cruz",
		Email: "delacruzportorrealsamueldavid@gmail.com",
	}
	config.Info.Description = `## 📘 Descripción de la API

Esta API proporciona acceso estructurado al texto bíblico de la **Reina-Valera 1960 (RV1960)**. Permite consultar libros, capítulos y versículos específicos de la Biblia, facilitando la navegación por las Escrituras de manera programática. Está pensada para ser utilizada por aplicaciones web, móviles o sistemas que necesiten integrar o mostrar contenido bíblico de forma precisa y eficiente.

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
		config.OpenAPI.Servers = []*huma.Server{
			{
				URL:         serverUrl,
				Description: "API URL",
			},
		}
	}
	api := humachi.New(router, config)

	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/books",
		Summary:     "Obtener todos los libros de la Biblia (RV1960)",
		Description: "Devuelve la lista completa de libros de la Biblia en la versión Reina Valera 1960, incluyendo información del testamento y los capítulos correspondientes.",
		Tags:        []string{"Books"},
	}, func(ctx context.Context, i *struct {
		// Version selects the translation; empty means the shipped one.
		Version string `query:"version" doc:"Traducción a leer: RVR1960 (por defecto), LBLA o NVI"`
	}) (*ListResponse[Book], error) {
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
		Summary:     "Obtener un libro específico (RV1960)",
		Description: "Devuelve los detalles de un libro de la Biblia en la versión Reina Valera 1960 a partir de su ID, incluyendo los capítulos que lo componen.",
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
		Path:        "/api/books/{bookId}/verses/from/{startChapterNumber}/to/{endChapterNumber}/verse/{endVerseNumber}",
		Summary:     "Obtener versículos entre capítulos (límite por versículo final)",
		Description: "Devuelve todos los versículos desde un capítulo inicial hasta un capítulo final, incluyendo solo hasta el versículo especificado en el último capítulo.",
		Tags:        []string{"Verses"},
	}, func(ctx context.Context, input *ChapterToChapterVersesRequest) (*ListResponse[Verse], error) {
		db, versionErr := store.pick(input.Version)
		if versionErr != nil {
			return nil, versionErr
		}
		if err := requireBook(input.BookId); err != nil {
			return nil, err
		}
		results := []Verse{}
		err := db.Select(&results, `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber FROM verses WHERE chapterId LIKE ? AND chapterNumber between ? AND ?  ORDER BY chapterNumber, verseNumber`, input.BookId+".%", input.StartChapterNumber, input.EndChapterNumber)
		if err != nil {
			return nil, dbError("error while getting verses from DB", err)
		}
		lastVerseIndex := slices.IndexFunc(results, func(verse Verse) bool {
			return verse.ChapterNumber == int(input.EndChapterNumber) && verse.VerseNumber == int(input.EndVerseNumber)
		})
		if lastVerseIndex == -1 {
			return nil, huma.Error404NotFound(fmt.Sprintf("verse not found: %s.%d.%d", input.BookId, input.EndChapterNumber, input.EndVerseNumber))
		}
		results = results[:lastVerseIndex+1]
		results = paginate(results, input.PaginationRequest)
		return &ListResponse[Verse]{
			Body: results,
		}, nil
	})
	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/books/{bookId}/verses/from/{startChapterNumber}/verse/{startVerseNumber}/to/{endChapterNumber}/verse/{endVerseNumber}",
		Summary:     "Obtener versículos entre capítulo y versículo inicial y final",
		Description: "Devuelve los versículos que se encuentran entre un capítulo y versículo inicial y un capítulo y versículo final, respetando ambos límites.",
		Tags:        []string{"Verses"},
	}, func(ctx context.Context, input *VerseRangeRequest) (*ListResponse[Verse], error) {
		db, versionErr := store.pick(input.Version)
		if versionErr != nil {
			return nil, versionErr
		}
		if err := requireBook(input.BookId); err != nil {
			return nil, err
		}
		results := []Verse{}
		err := db.Select(&results, `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber FROM verses WHERE chapterId LIKE ? AND chapterNumber between ? AND ?  ORDER BY chapterNumber, verseNumber`, input.BookId+".%", input.StartChapterNumber, input.EndChapterNumber)
		if err != nil {
			return nil, dbError("error while getting verses from DB", err)
		}
		startVerseIndex := slices.IndexFunc(results, func(verse Verse) bool {
			return verse.ChapterNumber == int(input.StartChapterNumber) && verse.VerseNumber == int(input.StartVerseNumber)
		})
		lastVerseIndex := slices.IndexFunc(results, func(verse Verse) bool {
			return verse.ChapterNumber == int(input.EndChapterNumber) && verse.VerseNumber == int(input.EndVerseNumber)
		})
		if startVerseIndex == -1 {
			return nil, huma.Error404NotFound(fmt.Sprintf("verse not found: %s.%d.%d", input.BookId, input.StartChapterNumber, input.StartVerseNumber))
		}
		if lastVerseIndex == -1 {
			return nil, huma.Error404NotFound(fmt.Sprintf("verse not found: %s.%d.%d", input.BookId, input.EndChapterNumber, input.EndVerseNumber))
		}
		if lastVerseIndex < startVerseIndex {
			return nil, huma.Error422UnprocessableEntity("endVerseNumber cannot be less than startVerseNumber")
		}
		results = results[startVerseIndex : lastVerseIndex+1]
		results = paginate(results, input.PaginationRequest)
		return &ListResponse[Verse]{
			Body: results,
		}, nil
	})

	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/books/{bookId}/verses/from/chapter/{startChapterNumber}/to/chapter/{endChapterNumber}",
		Summary:     "Obtener versículos entre capítulos",
		Description: "Devuelve todos los versículos que se encuentran entre dos capítulos específicos del mismo libro, sin límite por número de versículo.",
		Tags:        []string{"Verses"},
	}, func(ctx context.Context, input *ChapterRangeRequest) (*ListResponse[Verse], error) {
		db, versionErr := store.pick(input.Version)
		if versionErr != nil {
			return nil, versionErr
		}
		if err := requireBook(input.BookId); err != nil {
			return nil, err
		}
		results := []Verse{}
		query := `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber FROM verses WHERE chapterId LIKE ? AND chapterNumber BETWEEN ? AND ? ORDER BY chapterNumber, verseNumber`
		query, args := paginateSQL(query, []any{input.BookId + ".%", input.StartChapterNumber, input.EndChapterNumber}, input.PaginationRequest)
		err := db.Select(&results, query, args...)
		if err != nil {
			return nil, dbError("error while getting verses from DB", err)
		}
		return &ListResponse[Verse]{
			Body: results,
		}, nil
	})

	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/books/{bookId}/verses/chapter/{chapterNumber}",
		Summary:     "Obtener versículos por capítulo",
		Description: "Devuelve todos los versículos de un capítulo específico de un libro de la Biblia en la versión Reina Valera 1960.",
		Tags:        []string{"Verses"},
	}, func(ctx context.Context, input *VersesByChapterIdRequest) (*ListResponse[Verse], error) {
		db, versionErr := store.pick(input.Version)
		if versionErr != nil {
			return nil, versionErr
		}
		if err := requireBook(input.BookId); err != nil {
			return nil, err
		}
		verses := []Verse{}
		query := `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber FROM verses WHERE chapterId = ? ORDER BY verseNumber`
		query, args := paginateSQL(query, []any{fmt.Sprintf("%s.%d", input.BookId, input.ChapterNumber)}, input.PaginationRequest)
		err := db.Select(&verses, query, args...)
		if err != nil {
			return nil, dbError("error while getting verses from DB", err)
		}

		return &ListResponse[Verse]{
			Body: verses,
		}, nil
	})

	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/books/{bookId}/verses/chapter/{chapterNumber}/verse/{verseNumber}",
		Summary:     "Obtener un versículo específico",
		Description: "Devuelve un versículo específico de un libro a partir del número de capítulo y el número de versículo.",
		Tags:        []string{"Verses"},
	}, func(ctx context.Context, input *VerseRequest) (*SingleResponse[Verse], error) {
		db, versionErr := store.pick(input.Version)
		if versionErr != nil {
			return nil, versionErr
		}
		if err := requireBook(input.BookId); err != nil {
			return nil, err
		}
		verse := Verse{}
		verseId := fmt.Sprintf("%s.%d.%d", input.BookId, input.ChapterNumber, input.VerseNumber)
		err := db.Get(&verse, `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber FROM verses WHERE id = ?`, verseId)
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, dbError("error while getting verse from DB", err)
			}
			return nil, huma.Error404NotFound(fmt.Sprintf("verse not found: %s.%d", input.BookId, input.ChapterNumber))
		}
		return &SingleResponse[Verse]{
			Body: verse,
		}, nil
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
		Description: "Devuelve todos los versículos que contengan el texto especificado.",
		Tags:        []string{"Verses"},
	}, func(ctx context.Context, input *SearchRequest) (*ListResponse[Verse], error) {
		db, versionErr := store.pick(input.Version)
		if versionErr != nil {
			return nil, versionErr
		}
		verses := []Verse{}
		query := `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber FROM verses WHERE cleanTextAscii like ? ESCAPE '\'`
		query, args := paginateSQL(query, []any{"%" + escapeLike(removeAccents(input.Query)) + "%"}, input.PaginationRequest)
		err := db.Select(&verses, query, args...)
		if err != nil {
			return nil, dbError("error while getting verses from DB", err)
		}

		return &ListResponse[Verse]{
			Body: verses,
		}, nil
	})
	return router
}
