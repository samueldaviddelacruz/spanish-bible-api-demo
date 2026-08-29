package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	BookId string `path:"bookId" enum:"spa-RVR1960:Gen,spa-RVR1960:Exod,spa-RVR1960:Lev,spa-RVR1960:Num,spa-RVR1960:Deut,spa-RVR1960:Josh,spa-RVR1960:Judg,spa-RVR1960:Ruth,spa-RVR1960:1Sam,spa-RVR1960:2Sam,spa-RVR1960:1Kgs,spa-RVR1960:2Kgs,spa-RVR1960:1Chr,spa-RVR1960:2Chr,spa-RVR1960:Ezra,spa-RVR1960:Neh,spa-RVR1960:Esth,spa-RVR1960:Job,spa-RVR1960:Ps,spa-RVR1960:Prov,spa-RVR1960:Eccl,spa-RVR1960:Song,spa-RVR1960:Isa,spa-RVR1960:Jer,spa-RVR1960:Lam,spa-RVR1960:Ezek,spa-RVR1960:Dan,spa-RVR1960:Hos,spa-RVR1960:Joel,spa-RVR1960:Amos,spa-RVR1960:Obad,spa-RVR1960:Jonah,spa-RVR1960:Mic,spa-RVR1960:Nah,spa-RVR1960:Hab,spa-RVR1960:Zeph,spa-RVR1960:Hag,spa-RVR1960:Zech,spa-RVR1960:Mal,spa-RVR1960:Matt,spa-RVR1960:Mark,spa-RVR1960:Luke,spa-RVR1960:John,spa-RVR1960:Acts,spa-RVR1960:Rom,spa-RVR1960:1Cor,spa-RVR1960:2Cor,spa-RVR1960:Gal,spa-RVR1960:Eph,spa-RVR1960:Phil,spa-RVR1960:Col,spa-RVR1960:1Thess,spa-RVR1960:2Thess,spa-RVR1960:1Tim,spa-RVR1960:2Tim,spa-RVR1960:Titus,spa-RVR1960:Phlm,spa-RVR1960:Heb,spa-RVR1960:Jas,spa-RVR1960:1Pet,spa-RVR1960:2Pet,spa-RVR1960:1John,spa-RVR1960:2John,spa-RVR1960:3John,spa-RVR1960:Jude,spa-RVR1960:Rev" doc:"Identificador del libro bíblico (ej: 'spa-RVR1960:Gen')"`
}

type PaginationRequest struct {
	Limit  uint `query:"limit" doc:"Número máximo de versículos a devolver (opcional, 0 u omitido = sin límite)"`
	Offset uint `query:"offset" doc:"Cantidad de versículos a omitir (opcional)"`
}

type VersesByChapterIdRequest struct {
	BookRequest
	PaginationRequest
	ChapterNumber uint `path:"chapterNumber" required:"true" doc:"Número del capítulo del cual obtener los versículos"`
}

type VerseRequest struct {
	BookRequest
	ChapterNumber uint `path:"chapterNumber" required:"true" doc:"Número del capítulo que contiene el versículo"`
	VerseNumber   uint `path:"verseNumber" required:"true" doc:"Número del versículo a obtener"`
}

type SearchRequest struct {
	Query string `query:"q" required:"true" doc:"texto o termino a buscar"`
	PaginationRequest
}

type ChapterToChapterVersesRequest struct {
	BookRequest
	PaginationRequest
	StartChapterNumber uint `path:"startChapterNumber" required:"true" doc:"Capítulo inicial del rango"`
	EndChapterNumber   uint `path:"endChapterNumber" required:"true" doc:"Capítulo final del rango"`
	EndVerseNumber     uint `path:"endVerseNumber" required:"true" doc:"Último versículo a incluir del capítulo final"`
}

type VerseRangeRequest struct {
	BookRequest
	PaginationRequest
	StartChapterNumber uint `path:"startChapterNumber" required:"true" doc:"Capítulo inicial"`
	StartVerseNumber   uint `path:"startVerseNumber" required:"true" doc:"Versículo inicial dentro del capítulo inicial"`
	EndChapterNumber   uint `path:"endChapterNumber" required:"true" doc:"Capítulo final"`
	EndVerseNumber     uint `path:"endVerseNumber" required:"true" doc:"Versículo final dentro del capítulo final"`
}

type ChapterRangeRequest struct {
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
func main() {
	// Create a new router & API
	db, err := sqlx.Open("sqlite", "Bible.db")
	if err != nil {
		log.Fatal("error opening DB")
	}
	defer db.Close()
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

	router := newRouter(db)

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

func newRouter(db *sqlx.DB) *chi.Mux {
	router := chi.NewMux()

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
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

Esta API está centrada en la versión **Reina-Valera 1960**.  
No contiene comentarios, notas teológicas ni versiones alternativas del texto.
`
	env := os.Getenv("GO_ENV")

	if env == "PROD" || env == "PRODUCTION" {
		hostUrl := os.Getenv("HOST_URL")
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
	}, func(ctx context.Context, i *struct{}) (*ListResponse[Book], error) {
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
		Path:        "/api/verses/search",
		Summary:     "Buscar dentro de los versiculos de la biblia",
		Description: "Devuelve todos los versículos que contengan el texto especificado.",
		Tags:        []string{"Verses"},
	}, func(ctx context.Context, input *SearchRequest) (*ListResponse[Verse], error) {
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
