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
	BookId string `path:"bookId" enum:"spa-RVR1960:Gen,spa-RVR1960:Exod,spa-RVR1960:Lev,spa-RVR1960:Num,spa-RVR1960:Deut,spa-RVR1960:Josh,spa-RVR1960:Judg,spa-RVR1960:Ruth,spa-RVR1960:1Sam,spa-RVR1960:2Sam,spa-RVR1960:1Kgs,spa-RVR1960:2Kgs,spa-RVR1960:1Chr,spa-RVR1960:2Chr,spa-RVR1960:Ezra,spa-RVR1960:Neh,spa-RVR1960:Esth,spa-RVR1960:Job,spa-RVR1960:Ps,spa-RVR1960:Prov,spa-RVR1960:Eccl,spa-RVR1960:Song,spa-RVR1960:Isa,spa-RVR1960:Jer,spa-RVR1960:Lam,spa-RVR1960:Ezek,spa-RVR1960:Dan,spa-RVR1960:Hos,spa-RVR1960:Joel,spa-RVR1960:Amos,spa-RVR1960:Obad,spa-RVR1960:Jonah,spa-RVR1960:Mic,spa-RVR1960:Nah,spa-RVR1960:Hab,spa-RVR1960:Zeph,spa-RVR1960:Hag,spa-RVR1960:Zech,spa-RVR1960:Mal,spa-RVR1960:Matt,spa-RVR1960:Mark,spa-RVR1960:Luke,spa-RVR1960:John,spa-RVR1960:Acts,spa-RVR1960:Rom,spa-RVR1960:1Cor,spa-RVR1960:2Cor,spa-RVR1960:Gal,spa-RVR1960:Eph,spa-RVR1960:Phil,spa-RVR1960:Col,spa-RVR1960:1Thess,spa-RVR1960:2Thess,spa-RVR1960:1Tim,spa-RVR1960:2Tim,spa-RVR1960:Titus,spa-RVR1960:Phlm,spa-RVR1960:Heb,spa-RVR1960:Jas,spa-RVR1960:1Pet,spa-RVR1960:2Pet,spa-RVR1960:1John,spa-RVR1960:2John,spa-RVR1960:3John,spa-RVR1960:Jude,spa-RVR1960:Rev"`
}
type VersesByChapterIdRequest struct {
	BookRequest
	ChapterNumber string `path:"chapterNumber" required:"true"`
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

	router := chi.NewMux()
	api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))
	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/books",
		Summary:     "Get a List of RV1960 books",
		Description: "Get a List of Reina Valera 1960 Bible's books",
		Tags:        []string{"Books"},
	}, func(ctx context.Context, i *struct{}) (*ListResponse[Book], error) {
		books := []Book{}
		chapters := []Chapter{}
		err := db.Select(&books, `SELECT id, name, "order", testament FROM books`)
		if err != nil {

			return nil, fmt.Errorf("error while getting books from DB: %v", err)

		}
		err = db.Select(&chapters, "SELECT * FROM chapters")
		if err != nil {
			return nil, fmt.Errorf("error while getting chapters from DB: %v", err)
		}
		slices.SortFunc(books, func(c1 Book, c2 Book) int {
			return c1.Order - c2.Order
		})
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
		Summary:     "Get a a book from RV1960 by its id",
		Description: "Get a book from Reina Valera 1960 Bible by its id",
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
		err = db.Select(&book.Chapters, "SELECT * FROM chapters WHERE id like ?", "%"+book.ID+"%")
		if err != nil {
			return nil, fmt.Errorf("error while getting chapters from DB: %v", err)
		}
		slices.SortFunc(book.Chapters, func(c1 Chapter, c2 Chapter) int {
			return c1.Chapter - c2.Chapter
		})

		return &SingleResponse[Book]{
			Body: book,
		}, nil
	})

	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/books/{bookId}/verses/{chapterNumber}",
		Summary:     "Get verses by chapter id",
		Description: "Get verses by book chapter number",
		Tags:        []string{"Verses"},
	}, func(ctx context.Context, input *VersesByChapterIdRequest) (*ListResponse[Verse], error) {
		verses := []Verse{}
		err := db.Select(&verses, `SELECT id,chapterId,cleanText,reference,"text",chapterNumber,verseNumber FROM verses WHERE chapterId = ?`, fmt.Sprintf("%s.%s", input.BookId, input.ChapterNumber))
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, fmt.Errorf("error while getting verses from DB: %v", err)
			}
			return nil, huma.Error404NotFound(fmt.Sprintf("verses not found: %s.%s", input.BookId, input.ChapterNumber))
		}
		slices.SortFunc(verses, func(c1 Verse, c2 Verse) int {
			return c1.VerseNumber - c2.VerseNumber
		})
		return &ListResponse[Verse]{
			Body: verses,
		}, nil
	})

	// Start the server!
	fmt.Println("Starting server on port 8888 ")
	http.ListenAndServe("127.0.0.1:8888", router)
}
