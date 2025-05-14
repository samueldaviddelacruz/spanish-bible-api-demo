((data) => {
  const { sqliteDb } = require("./sqliteDb");

  data.getBibleBooks = async () => {
    try {
      const books = sqliteDb.prepare("SELECT * FROM books").all();
      for (const book of books) {
        const chapters = sqliteDb
          .prepare("SELECT * FROM chapters WHERE id like ?")
          .all(`%${book.id}%`);
        book.chapters = chapters.sort((a, b) => a.chapter - b.chapter);
      }
      return books;
    } catch (err) {
      throw err;
    }
  };

  data.getBibleBookById = async (id) => {
    try {
      const book = sqliteDb.prepare("SELECT * FROM books WHERE id = ?").get(id);
      return book;
    } catch (err) {
      throw err;
    }
  };

  data.getVersesByChapterNumber = async (bookId, chapterNumber) => {
    try {
      const verses = sqliteDb
        .prepare("SELECT * FROM verses WHERE chapterId = ?")
        .all(`${bookId}.${chapterNumber}`);
      if (!verses) {
        return [];
      }
      return verses;
    } catch (err) {
      return [];
    }
  };

  data.getVerseByChapterAndVerseNumber = async (bookId, verseRange) => {
    try {
      const [chapterNumber, verseNumber] = verseRange.split(":");
      const chapterId = `${bookId}.${chapterNumber}`;
      const verseId = `${chapterId}.${verseNumber}`;
      const verse = sqliteDb
        .prepare("SELECT * FROM verses WHERE id = ?")
        .get(verseId);
      return [verse];
    } catch (err) {
      console.log(err);
      return [];
    }
  };

  data.getVersesFromChapterUntilVerse = async (bookId, verseRange) => {
    try {
      console.log("verseRange", verseRange);
      let [startChapterNumber, endChapterAndEndVerse] = verseRange.split("-");
      let [endChapterNumber, endVerseNumber] = endChapterAndEndVerse.split(":");
      const startChapterVerses = sqliteDb
        .prepare(
          `SELECT * FROM verses WHERE chapterId LIKE ? AND chapterNumber = ?`
        )
        .all(`%${bookId}%`, startChapterNumber);
      const endChapterVerses = sqliteDb
        .prepare(
          `SELECT * FROM verses WHERE chapterId LIKE ? AND chapterNumber = ? AND verseNumber <= ?`
        )
        .all(`%${bookId}%`, endChapterNumber, endVerseNumber);
      if (!startChapterVerses || !endChapterVerses) {
        return [];
      }
      const result = [
        ...startChapterVerses.sort((a,b) => a.verseNumber - b.verseNumber),
        ...endChapterVerses.sort((a,b) => a.verseNumber - b.verseNumber),
      ];

      return result;

    } catch (err) {
      console.log(err);
      return [];
    }
  };

  data.getVersesFromVerseToVerse = async (bookId, verseRange) => {
    try {
      let [startChapterAndVerse, endChapterAndVerse] = verseRange.split("-");
      let [startChapterNumber, startVerseNumber] =
        startChapterAndVerse.split(":");
      let [endChapterNumber, endVerseNumber] = endChapterAndVerse.split(":");

     const startVerses = sqliteDb
        .prepare(
          `SELECT * FROM verses WHERE chapterId = ? AND chapterNumber = ? AND verseNumber >= ?`
        )
        .all(`${bookId}.${startChapterNumber}`, startChapterNumber, startVerseNumber);

      const endVerses = sqliteDb
        .prepare(
          `SELECT * FROM verses WHERE chapterId LIKE ? AND chapterNumber = ? AND verseNumber <= ?`
        )
        .all(`${bookId}.${endChapterNumber}`, endChapterNumber, endVerseNumber);

      if (!startVerses || !endVerses) {
        return [];
      }
      const result = [
        ...startVerses.sort((a, b) => a.verseNumber - b.verseNumber),
        ...endVerses.sort((a, b) => a.verseNumber - b.verseNumber),
      ];
      return result
    } catch (err) {
      console.log(err);
      return [];
    }
  };
  data.getVersesFromChapterToChapter = async (bookId, verseRange) => {
    try {
      let [startChapterNumber, endChapterNumber] = verseRange.split("-");
      const initialChapter = sqliteDb
        .prepare("SELECT * FROM chapters WHERE id = ?")
        .get(`${bookId}.${startChapterNumber}`);
      const endChapter = sqliteDb
        .prepare("SELECT * FROM chapters WHERE id = ?")
        .get(`${bookId}.${endChapterNumber}`);
      const initialChapterLastVerseNumber =
        +initialChapter.osis_end.split(".")[2];
      const endChapterLastVerseNumber = +endChapter.osis_end.split(".")[2];
      let startRangeVerses = [];
      let endRangeVerses = [];
      let sortingMap = {};

      for (let i = 1; i <= initialChapterLastVerseNumber; i++) {
        startRangeVerses.push(`${bookId}.${initialChapter.chapter}.${i}`);
      }
      for (let i = 1; i <= endChapterLastVerseNumber; i++) {
        endRangeVerses.push(`${bookId}.${endChapter.chapter}.${i}`);
      }

      const versesToLookUp = [...startRangeVerses, ...endRangeVerses];
      for (let i = 0; i < versesToLookUp.length; i++) {
        sortingMap[versesToLookUp[i]] = i;
      }
      const verses = sqliteDb
        .prepare(
          `SELECT * FROM verses WHERE id IN (${versesToLookUp
            .map((v) => "?")
            .join(",")})`
        )
        .all(...versesToLookUp);
      return verses.sort((a, b) => sortingMap[a.id] - sortingMap[b.id]);
    } catch (err) {
      console.log(err);
      throw err;
    }
  };

  data.addChapterNumberAndVerseNumber = async () => {
    try {
      const verses = sqliteDb.prepare("SELECT * FROM verses").all();

      for (const verse of verses) {
        //spa-RVR1960:1Cor.11.26
        const [_, chapterNumber, verseNumber] = verse.id
          .split(":")[1]
          .split(".");
        //update the chapterNumber and verseNumber
        sqliteDb
          .prepare(
            `UPDATE verses SET chapterNumber = ?, verseNumber = ? WHERE id = ?`
          )
          .run(chapterNumber, verseNumber, verse.id);
      }
    } catch (err) {}
  };
})(module.exports);
