((data) => {
  const { sqliteDb } = require("./sqliteDb");

  data.getBibleBooks = async () => {
    try {
      const books = sqliteDb.prepare("SELECT * FROM books").all();
      for(const book of books){
       const chapters = sqliteDb.prepare("SELECT * FROM chapters WHERE id like ?").all(`%${book.id}%`);
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
      let [startChapterNumber, endChapterAndEndVerse] = verseRange.split("-");
      let [endChapterNumber, endVerseNumber] = endChapterAndEndVerse.split(":");
      const startChapter = sqliteDb
        .prepare("SELECT * FROM chapters WHERE id = ?")
        .get(`${bookId}.${startChapterNumber}`);
      const endChapter = sqliteDb
        .prepare("SELECT * FROM chapters WHERE id = ?")
        .get(`${bookId}.${endChapterNumber}`);
      const startChapterLastVerseNumber = +startChapter.osis_end.split(".")[2];
      let startRangeVerses = [];
      let endRangeVerses = [];
      let sortingMap = {};
      for (let i = 1; i <= startChapterLastVerseNumber; i++) {
        startRangeVerses.push(`${bookId}.${startChapter.chapter}.${i}`);
      }
      for (let i = 1; i <= endVerseNumber; i++) {
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
      return [];
    }
  };

  data.getVersesFromVerseToVerse = async (bookId, verseRange) => {
    try {
      let [startChapterAndVerse, endChapterAndVerse] = verseRange.split("-");
      let [startChapterNumber, startVerseNumber] =
        startChapterAndVerse.split(":");
      let [endChapterNumber, endVerseNumber] = endChapterAndVerse.split(":");
   
      if (startChapterNumber === endChapterNumber) {
        let verseRange = [];
        let sortingMap = {};
        for (let i = startVerseNumber; i <= endVerseNumber; i++) {
          verseRange.push(`${bookId}.${startChapterNumber}.${i}`);
          sortingMap[`${bookId}.${startChapterNumber}.${i}`] = i;
        }
        const verses = sqliteDb
          .prepare(
            `SELECT * FROM verses WHERE id IN (${verseRange
              .map((v) => "?")
              .join(",")})`
          )
          .all(...verseRange);
        return verses.sort((a, b) => sortingMap[a.id] - sortingMap[b.id]);
      }
      if (endChapterNumber > startChapterNumber) {
        let startRangeVerses = [];
        let endRangeVerses = [];
        let sortingMap = {};
        let sortCounter = 0;
        const startChapter = sqliteDb
          .prepare("SELECT * FROM chapters WHERE id = ?")
          .get(`${bookId}.${startChapterNumber}`);
        const startChapterLastVerseNumber = +startChapter.osis_end.split(".")[2];
        for (let i = startVerseNumber; i <= startChapterLastVerseNumber; i++, sortCounter++) {
          startRangeVerses.push(`${bookId}.${startChapter.chapter}.${i}`);
          sortingMap[`${bookId}.${startChapter.chapter}.${i}`] = sortCounter;
        }
        for (let i = 1; i <= endVerseNumber; i++, sortCounter++) {
          endRangeVerses.push(`${bookId}.${endChapterNumber}.${i}`);
          sortingMap[`${bookId}.${endChapterNumber}.${i}`] = sortCounter;
        }
        const verses = sqliteDb.prepare(`SELECT * FROM verses WHERE id IN (${[...startRangeVerses, ...endRangeVerses].map((v) => "?").join(",")})`).all(...[...startRangeVerses, ...endRangeVerses]);
        return verses.sort((a, b) => sortingMap[a.id] - sortingMap[b.id]);
      }

      return [];

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
})(module.exports);
