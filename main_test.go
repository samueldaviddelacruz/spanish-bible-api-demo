package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

type seedVerse struct {
	id            string
	chapterID     string
	cleanText     string
	reference     string
	chapterNumber int
	verseNumber   int
}

var seedVerses = []seedVerse{
	{"spa-RVR1960:Gen.1.1", "spa-RVR1960:Gen.1", "En el principio creó Dios los cielos y la tierra.", "Génesis 1:1", 1, 1},
	{"spa-RVR1960:Gen.1.2", "spa-RVR1960:Gen.1", "Y la tierra estaba desordenada y vacía.", "Génesis 1:2", 1, 2},
	{"spa-RVR1960:Gen.1.3", "spa-RVR1960:Gen.1", "Y dijo Dios: Sea la luz; y la luz fue.", "Génesis 1:3", 1, 3},
	{"spa-RVR1960:Gen.2.1", "spa-RVR1960:Gen.2", "Fueron, pues, acabados los cielos y la tierra.", "Génesis 2:1", 2, 1},
	{"spa-RVR1960:Gen.2.2", "spa-RVR1960:Gen.2", "Y acabó Dios en el día séptimo la obra que hizo.", "Génesis 2:2", 2, 2},
	{"spa-RVR1960:Gen.3.1", "spa-RVR1960:Gen.3", "Cien por ciento 100% y guión_bajo.", "Génesis 3:1", 3, 1},
	{"spa-RVR1960:Exod.1.1", "spa-RVR1960:Exod.1", "Estos son los nombres de los hijos de Israel.", "Éxodo 1:1", 1, 1},
	{"notspa-RVR1960:Gen.1.1", "notspa-RVR1960:Gen.1", "Versículo intruso.", "Intruso 1:1", 1, 1},
}

func setupTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := []string{
		`CREATE TABLE books (_id TEXT, id TEXT PRIMARY KEY, name TEXT, "order" INTEGER, testament TEXT)`,
		`CREATE TABLE chapters (chapter INT, id TEXT PRIMARY KEY, osis_end TEXT)`,
		`CREATE TABLE verses (_id TEXT, chapterId TEXT, cleanText TEXT, id TEXT, reference TEXT, "text" TEXT, chapterNumber INTEGER, verseNumber INTEGER, cleanTextAscii TEXT)`,
	}
	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}

	books := []struct {
		id, name  string
		order     int
		testament string
	}{
		{"spa-RVR1960:Exod", "Éxodo", 2, "OT"},
		{"spa-RVR1960:Gen", "Génesis", 1, "OT"},
	}
	for _, b := range books {
		if _, err := db.Exec(`INSERT INTO books (id, name, "order", testament) VALUES (?, ?, ?, ?)`, b.id, b.name, b.order, b.testament); err != nil {
			t.Fatalf("seed book %s: %v", b.id, err)
		}
	}

	chapters := []struct {
		chapter int
		id      string
	}{
		{2, "spa-RVR1960:Gen.2"},
		{1, "spa-RVR1960:Gen.1"},
		{3, "spa-RVR1960:Gen.3"},
		{1, "spa-RVR1960:Exod.1"},
		{1, "notspa-RVR1960:Gen.1"},
	}
	for _, c := range chapters {
		if _, err := db.Exec(`INSERT INTO chapters (chapter, id, osis_end) VALUES (?, ?, ?)`, c.chapter, c.id, c.id); err != nil {
			t.Fatalf("seed chapter %s: %v", c.id, err)
		}
	}

	for _, v := range seedVerses {
		if _, err := db.Exec(`INSERT INTO verses (id, chapterId, cleanText, cleanTextAscii, reference, "text", chapterNumber, verseNumber) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			v.id, v.chapterID, v.cleanText, removeAccents(v.cleanText), v.reference, v.cleanText, v.chapterNumber, v.verseNumber); err != nil {
			t.Fatalf("seed verse %s: %v", v.id, err)
		}
	}
	return db
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	t.Setenv("GO_ENV", "LOCAL")
	srv := httptest.NewServer(newRouter(setupTestDB(t)))
	t.Cleanup(srv.Close)
	return srv
}

func doGet(t *testing.T, url string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body for GET %s: %v", url, err)
	}
	return resp.StatusCode, body
}

func mustUnmarshal(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("unmarshal %s: %v", body, err)
	}
}

func verseIDs(verses []Verse) []string {
	ids := make([]string, len(verses))
	for i, v := range verses {
		ids[i] = v.ID
	}
	return ids
}

func assertVerseIDs(t *testing.T, body []byte, want []string) []Verse {
	t.Helper()
	var verses []Verse
	mustUnmarshal(t, body, &verses)
	if got := verseIDs(verses); !slices.Equal(got, want) {
		t.Fatalf("verse ids = %v, want %v", got, want)
	}
	return verses
}

func TestRemoveAccents(t *testing.T) {
	cases := map[string]string{
		"Génesis": "Genesis",
		"creó":    "creo",
		"vacía":   "vacia",
		"señor":   "senor",
		"plain":   "plain",
		"":        "",
	}
	for in, want := range cases {
		if got := removeAccents(in); got != want {
			t.Errorf("removeAccents(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilter(t *testing.T) {
	got := Filter([]int{1, 2, 3, 4}, func(n int) bool { return n%2 == 0 })
	if want := []int{2, 4}; !slices.Equal(got, want) {
		t.Fatalf("Filter = %v, want %v", got, want)
	}
	all := Filter([]int{2, 4}, func(n int) bool { return n%2 == 0 })
	if want := []int{2, 4}; !slices.Equal(all, want) {
		t.Fatalf("Filter (all match) = %v, want %v", all, want)
	}
}

func TestHealth(t *testing.T) {
	srv := newTestServer(t)
	status, body := doGet(t, srv.URL+"/health")
	if status != http.StatusOK {
		t.Fatalf("health: status = %d, body = %s", status, body)
	}
}

func TestListBooks(t *testing.T) {
	srv := newTestServer(t)
	status, body := doGet(t, srv.URL+"/api/books")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	var books []Book
	mustUnmarshal(t, body, &books)
	if len(books) != 2 {
		t.Fatalf("got %d books, want 2", len(books))
	}
	if books[0].ID != "spa-RVR1960:Gen" || books[1].ID != "spa-RVR1960:Exod" {
		t.Fatalf("books not ordered by \"order\": %v", books)
	}
	if len(books[0].Chapters) != 3 {
		t.Fatalf("Genesis should have 3 chapters, got %d", len(books[0].Chapters))
	}
	if got := []string{books[0].Chapters[0].ID, books[0].Chapters[1].ID, books[0].Chapters[2].ID}; !slices.Equal(got, []string{"spa-RVR1960:Gen.1", "spa-RVR1960:Gen.2", "spa-RVR1960:Gen.3"}) {
		t.Fatalf("Genesis chapters not sorted: %v", got)
	}
	if len(books[1].Chapters) != 1 {
		t.Fatalf("Exodus should have 1 chapter, got %d", len(books[1].Chapters))
	}
}

func TestGetBook(t *testing.T) {
	srv := newTestServer(t)

	status, body := doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	var book Book
	mustUnmarshal(t, body, &book)
	if book.Name != "Génesis" || book.Testament != "OT" {
		t.Fatalf("unexpected book: %+v", book)
	}
	if len(book.Chapters) != 3 {
		t.Fatalf("got %d chapters, want 3", len(book.Chapters))
	}

	status, _ = doGet(t, srv.URL+"/api/books/spa-RVR1960:Rev")
	if status != http.StatusNotFound {
		t.Fatalf("missing book: status = %d, want 404", status)
	}

	status, _ = doGet(t, srv.URL+"/api/books/spa-RVR1960:Nope")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid book id: status = %d, want 422", status)
	}
}

func TestGetVersesByChapter(t *testing.T) {
	srv := newTestServer(t)

	status, body := doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/chapter/1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.1", "spa-RVR1960:Gen.1.2", "spa-RVR1960:Gen.1.3"})

	status, body = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/chapter/99")
	if status != http.StatusOK {
		t.Fatalf("missing chapter: status = %d, want 200", status)
	}
	assertVerseIDs(t, body, []string{})
}

func TestGetVerse(t *testing.T) {
	srv := newTestServer(t)

	status, body := doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/chapter/1/verse/1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	var verse Verse
	mustUnmarshal(t, body, &verse)
	if verse.ID != "spa-RVR1960:Gen.1.1" || verse.CleanText != "En el principio creó Dios los cielos y la tierra." {
		t.Fatalf("unexpected verse: %+v", verse)
	}

	status, _ = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/chapter/1/verse/99")
	if status != http.StatusNotFound {
		t.Fatalf("missing verse: status = %d, want 404", status)
	}
}

func TestVerseRange(t *testing.T) {
	srv := newTestServer(t)

	status, body := doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/1/verse/2/to/2/verse/1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.2", "spa-RVR1960:Gen.1.3", "spa-RVR1960:Gen.2.1"})

	status, _ = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/1/verse/99/to/2/verse/1")
	if status != http.StatusNotFound {
		t.Fatalf("missing start verse: status = %d, want 404 (used to panic)", status)
	}

	status, _ = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/1/verse/1/to/2/verse/99")
	if status != http.StatusNotFound {
		t.Fatalf("missing end verse: status = %d, want 404", status)
	}

	status, _ = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/1/verse/3/to/1/verse/1")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("inverted verse range: status = %d, want 422 (used to panic)", status)
	}

	status, _ = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/2/verse/1/to/1/verse/1")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("inverted chapter range: status = %d, want 422", status)
	}
}

func TestChapterToChapterVerses(t *testing.T) {
	srv := newTestServer(t)

	status, body := doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/1/to/2/verse/1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.1", "spa-RVR1960:Gen.1.2", "spa-RVR1960:Gen.1.3", "spa-RVR1960:Gen.2.1"})

	status, body = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/1/to/1/verse/2")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.1", "spa-RVR1960:Gen.1.2"})

	status, _ = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/1/to/2/verse/99")
	if status != http.StatusNotFound {
		t.Fatalf("missing end verse: status = %d, want 404 (used to return silent empty list)", status)
	}
}

func TestChapterRange(t *testing.T) {
	srv := newTestServer(t)

	status, body := doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/chapter/1/to/chapter/2")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{
		"spa-RVR1960:Gen.1.1", "spa-RVR1960:Gen.1.2", "spa-RVR1960:Gen.1.3",
		"spa-RVR1960:Gen.2.1", "spa-RVR1960:Gen.2.2",
	})

	status, body = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/chapter/5/to/chapter/6")
	if status != http.StatusOK {
		t.Fatalf("empty range: status = %d, want 200", status)
	}
	assertVerseIDs(t, body, []string{})

	status, _ = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/chapter/2/to/chapter/1")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("inverted chapter range: status = %d, want 422", status)
	}
}

func TestPagination(t *testing.T) {
	srv := newTestServer(t)

	status, body := doGet(t, srv.URL+"/api/verses/search?q=Dios&limit=2")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.1", "spa-RVR1960:Gen.1.3"})

	status, body = doGet(t, srv.URL+"/api/verses/search?q=Dios&limit=2&offset=2")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.2.2"})

	status, body = doGet(t, srv.URL+"/api/verses/search?q=Dios&offset=1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.3", "spa-RVR1960:Gen.2.2"})

	status, body = doGet(t, srv.URL+"/api/verses/search?q=Dios&offset=9")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{})

	status, body = doGet(t, srv.URL+"/api/verses/search?q=Dios&limit=0")
	if status != http.StatusOK {
		t.Fatalf("limit=0: status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.1", "spa-RVR1960:Gen.1.3", "spa-RVR1960:Gen.2.2"})

	status, body = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/chapter/1?limit=2&offset=1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.2", "spa-RVR1960:Gen.1.3"})

	status, body = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/chapter/1/to/chapter/2?limit=2&offset=1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.2", "spa-RVR1960:Gen.1.3"})

	status, body = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/1/verse/1/to/2/verse/2?limit=2&offset=2")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.3", "spa-RVR1960:Gen.2.1"})

	status, body = doGet(t, srv.URL+"/api/books/spa-RVR1960:Gen/verses/from/1/to/2/verse/2?limit=1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.1"})
}

func TestProdSchemaLink(t *testing.T) {
	t.Setenv("GO_ENV", "PROD")
	t.Setenv("HOST_URL", "https://gateway.example.com")
	srv := httptest.NewServer(newRouter(setupTestDB(t)))
	t.Cleanup(srv.Close)
	url := srv.URL + "/api/books/spa-RVR1960:Gen/verses/chapter/1/verse/1"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Forwarded-Host", "gateway.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var payload map[string]any
	mustUnmarshal(t, body, &payload)
	if got, _ := payload["$schema"].(string); got != "https://gateway.example.com/dev/schemas/Verse.json" {
		t.Fatalf("$schema = %q, want https://gateway.example.com/dev/schemas/Verse.json", got)
	}

	status, body := doGet(t, url)
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	var fallback map[string]any
	mustUnmarshal(t, body, &fallback)
	if got, _ := fallback["$schema"].(string); !strings.Contains(got, "/dev/schemas/Verse.json") {
		t.Fatalf("$schema without X-Forwarded-Host = %q, want it to contain /dev/schemas/Verse.json", got)
	}
}

func TestProdOpenAPIServers(t *testing.T) {
	t.Run("production sets servers from HOST_URL", func(t *testing.T) {
		t.Setenv("GO_ENV", "PRODUCTION")
		t.Setenv("HOST_URL", "https://gateway.example.com")
		srv := httptest.NewServer(newRouter(setupTestDB(t)))
		t.Cleanup(srv.Close)
		resp, err := http.Get(srv.URL + "/openapi.json")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		var spec struct {
			Servers []struct {
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"servers"`
		}
		mustUnmarshal(t, body, &spec)
		if len(spec.Servers) != 1 || spec.Servers[0].URL != "https://gateway.example.com/dev" {
			t.Fatalf("servers = %+v, want one entry with url https://gateway.example.com/dev", spec.Servers)
		}
	})

	t.Run("local dev omits servers", func(t *testing.T) {
		srv := newTestServer(t)
		resp, err := http.Get(srv.URL + "/openapi.json")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		var spec map[string]any
		mustUnmarshal(t, body, &spec)
		if servers, _ := spec["servers"].([]any); len(servers) != 0 {
			t.Fatalf("servers = %v, want none in local dev", servers)
		}
	})
}

func TestSearch(t *testing.T) {
	srv := newTestServer(t)

	status, body := doGet(t, srv.URL+"/api/verses/search?q=creo")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.1"})

	status, body = doGet(t, srv.URL+"/api/verses/search?q=creó")
	if status != http.StatusOK {
		t.Fatalf("accented query: status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.1"})

	status, body = doGet(t, srv.URL+"/api/verses/search?q=Dios")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.1.1", "spa-RVR1960:Gen.1.3", "spa-RVR1960:Gen.2.2"})

	status, body = doGet(t, srv.URL+"/api/verses/search?q=xyznotfound")
	if status != http.StatusOK {
		t.Fatalf("no matches: status = %d, want 200", status)
	}
	assertVerseIDs(t, body, []string{})

	status, body = doGet(t, srv.URL+"/api/verses/search?q=%25")
	if status != http.StatusOK {
		t.Fatalf("literal percent query: status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.3.1"})

	status, body = doGet(t, srv.URL+"/api/verses/search?q=_")
	if status != http.StatusOK {
		t.Fatalf("literal underscore query: status = %d, body = %s", status, body)
	}
	assertVerseIDs(t, body, []string{"spa-RVR1960:Gen.3.1"})
}

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"plain": "plain",
		"100%":  `100\%`,
		"a_b":   `a\_b`,
		`c\d`:   `c\\d`,
		`%\_`:   `\%\\\_`,
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Errorf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}
