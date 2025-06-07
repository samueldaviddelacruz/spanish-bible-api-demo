package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type Book struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Order     int       `json:"order"`
	Testament string    `json:"testament"`
	Chapters  []Chapter `json:"chapters"`
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

type VersesByChapterIdRequest struct {
	BookRequest
	ChapterNumber uint `path:"chapterNumber" required:"true" doc:"Número del capítulo del cual obtener los versículos"`
}

type VerseRequest struct {
	BookRequest
	ChapterNumber uint `path:"chapterNumber" required:"true" doc:"Número del capítulo que contiene el versículo"`
	VerseNumber   uint `path:"verseNumber" required:"true" doc:"Número del versículo a obtener"`
}

type ChapterToChapterVersesRequest struct {
	BookRequest
	StartChapterNumber uint `path:"startChapterNumber" required:"true" doc:"Capítulo inicial del rango"`
	EndChapterNumber   uint `path:"endChapterNumber" required:"true" doc:"Capítulo final del rango"`
	EndVerseNumber     uint `path:"endVerseNumber" required:"true" doc:"Último versículo a incluir del capítulo final"`
}

type VerseRangeRequest struct {
	BookRequest
	StartChapterNumber uint `path:"startChapterNumber" required:"true" doc:"Capítulo inicial"`
	StartVerseNumber   uint `path:"startVerseNumber" required:"true" doc:"Versículo inicial dentro del capítulo inicial"`
	EndChapterNumber   uint `path:"endChapterNumber" required:"true" doc:"Capítulo final"`
	EndVerseNumber     uint `path:"endVerseNumber" required:"true" doc:"Versículo final dentro del capítulo final"`
}

type ChapterRangeRequest struct {
	BookRequest
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
func main() {
	// Create a new router & API
	db, err := sqlx.Open("sqlite", "Bible.db")
	if err != nil {
		log.Fatal("error opening DB")
	}
	defer db.Close()

	router := chi.NewMux()
	api := humachi.New(router, huma.DefaultConfig("RV 1960 API", "1.0.0"))
	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/books",
		Summary:     "Obtener todos los libros de la Biblia (RV1960)",
		Description: "Devuelve la lista completa de libros de la Biblia en la versión Reina Valera 1960, incluyendo información del testamento y los capítulos correspondientes.",
		Tags:        []string{"Books"},
	}, func(ctx context.Context, i *struct{}) (*ListResponse[Book], error) {
		books := []Book{}
		chapters := []Chapter{}
		err := db.Select(&books, `SELECT id, name, "order", testament FROM books ORDER BY "order"`)
		if err != nil {

			return nil, fmt.Errorf("error while getting books from DB: %v", err)

		}
		err = db.Select(&chapters, "SELECT * FROM chapters")
		if err != nil {
			return nil, fmt.Errorf("error while getting chapters from DB: %v", err)
		}

		for i := range books {
			bookChapters := Filter(chapters, func(c Chapter) bool {
				return strings.Contains(c.ID, books[i].ID)
			})
			slices.SortFunc(bookChapters, func(c1 Chapter, c2 Chapter) int {
				return c1.Chapter - c2.Chapter
			})
			books[i].Chapters = append(books[i].Chapters, bookChapters...)
		}
		return &ListResponse[Book]{
			Body: books,
		}, nil
	})

	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/books/{bookId}",
		Summary:     "Obtener un libro específico (RV1960)",
		Description: "Devuelve los detalles de un libro de la Biblia en la versión Reina Valera 1960 a partir de su ID, incluyendo los capítulos que lo componen.",
		Tags:        []string{"Book"},
	}, func(ctx context.Context, input *BookRequest) (*SingleResponse[Book], error) {
		book := Book{}

		err := db.Get(&book, `SELECT id, name, "order", testament FROM books WHERE id = ?`, input.BookId)
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, fmt.Errorf("error while getting book from DB: %v", err)
			}
			return nil, huma.Error404NotFound(fmt.Sprintf("Book not found: %s", input.BookId))
		}
		err = db.Select(&book.Chapters, "SELECT * FROM chapters WHERE id like ? ORDER BY chapter", "%"+book.ID+"%")
		if err != nil {
			return nil, fmt.Errorf("error while getting chapters from DB: %v", err)
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
		startChapterVerses := []Verse{}
		endChapterVerses := []Verse{}
		results := []Verse{}
		err := db.Select(&startChapterVerses, `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber FROM verses WHERE chapterId = ? ORDER BY verseNumber`, fmt.Sprintf("%s.%d", input.BookId, input.StartChapterNumber))
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, fmt.Errorf("error while getting verses from DB: %v", err)
			}
			return nil, huma.Error404NotFound(fmt.Sprintf("verses not found: %s.%d", input.BookId, input.StartChapterNumber))
		}

		err = db.Select(&endChapterVerses, `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber FROM verses WHERE chapterId = ? AND verseNumber <= ? ORDER BY verseNumber`, fmt.Sprintf("%s.%d", input.BookId, input.EndChapterNumber), input.EndVerseNumber)
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, fmt.Errorf("error while getting verses from DB: %v", err)
			}
			return nil, huma.Error404NotFound(fmt.Sprintf("verses not found: %s.%d", input.BookId, input.EndChapterNumber))
		}

		results = append(results, startChapterVerses...)
		results = append(results, endChapterVerses...)
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
		startChapterVerses := []Verse{}
		endChapterVerses := []Verse{}
		results := []Verse{}
		err := db.Select(&startChapterVerses, `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber 
												FROM verses 
												WHERE chapterId = ? AND verseNumber >= ? ORDER BY verseNumber`,
			fmt.Sprintf("%s.%d", input.BookId, input.StartChapterNumber), input.StartVerseNumber)
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, fmt.Errorf("error while getting verses from DB: %v", err)
			}
			return nil, huma.Error404NotFound(fmt.Sprintf("verses not found: %s.%d", input.BookId, input.StartChapterNumber))
		}

		err = db.Select(&endChapterVerses, `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber 
											FROM verses 
											WHERE chapterId = ? AND verseNumber <= ? ORDER BY verseNumber`,
			fmt.Sprintf("%s.%d", input.BookId, input.EndChapterNumber), input.EndVerseNumber)

		if err != nil {
			if err != sql.ErrNoRows {
				return nil, fmt.Errorf("error while getting verses from DB: %v", err)
			}
			return nil, huma.Error404NotFound(fmt.Sprintf("verses not found: %s.%d", input.BookId, input.EndChapterNumber))
		}

		results = append(results, startChapterVerses...)
		results = append(results, endChapterVerses...)
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
		err := db.Select(&results, `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber 
									FROM verses WHERE chapterId LIKE ? 
									AND chapterNumber BETWEEN ? AND ? 
									ORDER BY chapterNumber,verseNumber`, "%"+input.BookId+"%", input.StartChapterNumber, input.EndChapterNumber)
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, fmt.Errorf("error while getting verses from DB: %v", err)
			}
			return nil, huma.Error404NotFound(fmt.Sprintf("verses not found: %s.%d", input.BookId, input.StartChapterNumber))
		}
		results = append(results, results...)
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
		err := db.Select(&verses, `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber FROM verses WHERE chapterId = ? ORDER BY verseNumber`, fmt.Sprintf("%s.%d", input.BookId, input.ChapterNumber))
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, fmt.Errorf("error while getting verses from DB: %v", err)
			}
			return nil, huma.Error404NotFound(fmt.Sprintf("verses not found: %s.%d", input.BookId, input.ChapterNumber))
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
				return nil, fmt.Errorf("error while getting verse from DB: %v", err)
			}
			return nil, huma.Error404NotFound(fmt.Sprintf("verse not found: %s.%d", input.BookId, input.ChapterNumber))
		}
		return &SingleResponse[Verse]{
			Body: verse,
		}, nil
	})

	// Start the server!
	fmt.Println("Starting server on port 8888 ")
	http.ListenAndServe("127.0.0.1:8888", router)
}
