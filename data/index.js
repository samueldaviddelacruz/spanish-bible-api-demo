(data => {
  const database = require("./database");

  data.getUser = async username => {
    try {
      const db = await database.getDb();

      const user = await db.usuarios.findOne({
        username: username.toUpperCase()
      });

      return user;
    } catch (err) {
      throw err;
    }
  };

  data.addUser = async user => {
    try {
      const db = await database.getDb();
      return await db.usuarios.insert(user);
    } catch (err) {
      console.log(err);
      throw err;
    }
  };

  data.addToFavorite = async (username, verseData, next) => {
    try {
      const db = await database.getDb();
      db.usuarios.update(
        { username: username },
        { $addToSet: { MyFavoriteVerses: verseData } },
        next
      );
    } catch (err) {
      next(err);
    }
  };

  data.removeFromFavorites = async (username, verseData) => {
    try {
      const db = await database.getDb();
      await db.usuarios.update(
        { username: username },
        { $pull: { MyFavoriteVerses: verseData } }
      );
    } catch (err) {
      console.log(err);
      throw err;
    }
  };

  data.getFavoriteVerses = async (username, next) => {
    try {
      const db = await database.getDb();

      const user = await db.usuarios.findOne({
        username: username.toUpperCase()
      });

      let verses = [];
      for (const fav of user.MyFavoriteVerses) {
        const verse = await data.getBibleVerseById(fav.verseId);
        verses.push(verse);
      }

      return verses;
    } catch (err) {
      console.log(err);
      throw err;
    }
  };

  data.getBibleBooks = async () => {
    try {
      const db = await database.getDb();

      const books = await db.bibleBooks
        .find({}, { projection: { _id: 0 } })
        .toArray();

      return books;
    } catch (err) {
      throw err;
    }
  };

  data.getBibleBookById = async id => {
    try {
      const db = await database.getDb();

      const book = await db.bibleBooks.findOne(
        { id },
        { projection: { _id: 0 } }
      );

      return book;
    } catch (err) {
      throw err;
    }
  };

  data.getVersesByChapterNumber = async (bookId, chapterNumber) => {
    try {
      const book = await data.getBibleBookById(bookId);
      const chapter = book.chapters.find(c => c.chapter === chapterNumber);
      if (!chapter) {
        return [];
      }
      return await data.getBibleVersesByChapterId(chapter.id);
    } catch (err) {
      return []
    }
  };

  data.getVerseByChapterAndVerseNumber = async (bookId, verseRange) => {
    try {
      const [chapterNumber, verseNumber] = verseRange.split(":");
      const book = await data.getBibleBookById(bookId);
      const chapter = book.chapters.find(c => c.chapter === chapterNumber);
      const verseId = `${chapter.id}.${verseNumber}`;
      const result = await data.getBibleVerseById(verseId);

      return [result];
    } catch (err) {
      return []
    }
  };

  data.getVersesFromChapterUntilVerse = async (bookId, verseRange) => {

    try {
      let [startChapterNumber, startVerseAndEndVerse] = verseRange.split("-");
      let [verseStart,verseEnd] = startVerseAndEndVerse.split(":");
        startChapterNumber = +startChapterNumber;
        verseStart = +verseStart;
        verseEnd = +verseEnd;
        const chaptersMap = await getBookChaptersMap(bookId);
        const chapter = chaptersMap[startChapterNumber];
        const verses = await data.getBibleVersesByChapterId(
          chapter.id
        );
        const versesMap = verses.reduce((acc, verse) => {
         
          return {...acc,[verse.id]:verse};
         }, {});
        let result = []
        for(let i = verseStart; i <= verseEnd; i++){
          const verse = versesMap[`${chapter.id}.${i}`];
          if(verse){
            result.push(versesMap[`${chapter.id}.${i}`])
          }
        }
        return result
    } catch (err) {
      console.log(err);
      return []
    }
  };
  data.getVersesFromVerseUntilChapter = async (bookId, verseRange) => {
    try {
      let [startChapterAndVerse, endChapterNumber] = verseRange.split("-");
      let [startChapterNumber, startVerseNumber] = startChapterAndVerse.split(
        ":"
      );
      endChapterNumber = +endChapterNumber;
      startChapterNumber = +startChapterNumber;
      startVerseNumber = +startVerseNumber;
      let result = [];
      let chaptersMap = await getBookChaptersMap(bookId);
      const { startChapter, endChapter } = await getStartAndEndChapters(
        bookId,
        startChapterNumber,
        endChapterNumber
      );

      if (startChapter && endChapter) {
        const firstBatchOfVerses = await data.getBibleVersesByChapterId(
          startChapter.id
        );
       
        

        result.push(...firstBatchOfVerses.slice(startVerseNumber - 1));

        let chapterIdList = getChapterIdList(
          startChapterNumber+1,
          endChapterNumber,
          chaptersMap
        );

        const verses = await findManyVersesByChapterIds(chapterIdList);
        result.push(...verses);

        return result
      }

      return [];
    } catch (err) {
      console.log(err);
      return []
    }
    
  };

  data.getVersesFromVerseToVerse = async (bookId, verseRange) => {
    try {
      let [startChapterAndVerse, endChapterAndVerse] = verseRange.split("-");
      let [startChapterNumber, startVerseNumber] = startChapterAndVerse.split(
        ":"
      );
      let [endChapterNumber, endVerseNumber] = endChapterAndVerse.split(":");
      startChapterNumber = +startChapterNumber;
      startVerseNumber = +startVerseNumber;
      endChapterNumber = +endChapterNumber;
      endVerseNumber = +endVerseNumber;
      const verses = await data.getVersesFromChapterToChapter(bookId, `${startChapterNumber}-${endChapterNumber}`);
      const startVerseIndex = verses.findIndex(verse => verse.id === `${bookId}.${startChapterNumber}.${startVerseNumber}`);
      const endVerseIndex = verses.findIndex(verse => verse.id === `${bookId}.${endChapterNumber}.${endVerseNumber}`);
      const result = verses.slice(startVerseIndex, endVerseIndex + 1);
      return result;
    } catch (err) {
      console.log(err);
      return []
    }
  };
  data.getVersesFromChapterToChapter = async (bookId, verseRange) => {
    try {
      let [startChapterNumber, endChapterNumber] = verseRange.split("-");
      startChapterNumber = +startChapterNumber;
      endChapterNumber = +endChapterNumber;   
      const chaptersMap = await getBookChaptersMap(bookId);
      const result = [];
      for(let i = startChapterNumber; i <= endChapterNumber; i++){
        const chapter = chaptersMap[i];
        const versesEnd = +chapter.osis_end.split(".")[2];
        const verses = await data.getBibleVersesByChapterId(chapter.id);
        const versesMap = verses.reduce((acc, verse) => {
          return {...acc,[verse.id]:verse};
         }, {});
         for(let i = 1; i <= versesEnd; i++){
          const verse = versesMap[`${chapter.id}.${i}`];
          if(verse){
            result.push(verse)
          }
         }
        result.push(...verses);
      }
      return result;
    } catch (err) {
      console.log(err);
      throw err;
    }
  };

  data.getBibleVersesByChapterId = async id => {
    try {
      const db = await database.getDb();

      const versesByChapterId = await db.bibleVerses
        .find({ chapterId: id }, { projection: { _id: 0 } })
        .toArray();

      return versesByChapterId;
    } catch (err) {
      throw err;
    }
  };
  data.getBibleVerseById = async id => {
    try {
      const db = await database.getDb();

      const verse = await db.bibleVerses.findOne(
        { id },
        { projection: { _id: 0 } }
      );

      return verse;
    } catch (err) {
      throw err;
    }
  };

  function getChapterIdList(startChapterNumber, endChapter, chaptersMap) {
    const chapterIdList = [];
    
    for (let i = +startChapterNumber; i <= +endChapter; i++) {
      const chapter = chaptersMap[`${i}`];
      if (chapter) {
        chapterIdList.push(chapter.id);
      }
    }
    console.log(chapterIdList);
    return chapterIdList;
  }

  async function findManyVersesByChapterIds(chapterIdList) {
    const db = await database.getDb();
    return await db.bibleVerses
      .find({ chapterId: { $in: chapterIdList } }, { projection: { _id: 0 } })
      .toArray();
  }

  async function getBookChaptersMap(bookId) {
    const book = await data.getBibleBookById(bookId);
    return book.chapters.reduce((acc, nextVal) => {
      return { ...acc, [nextVal.chapter]: nextVal };
    }, {});
  }
  async function getStartAndEndChapters(
    bookId,
    startChapterNumber,
    endChapterNumber
  ) {
    const chaptersMap = await getBookChaptersMap(bookId);
    const startChapter = chaptersMap[`${startChapterNumber}`];
    const endChapter = chaptersMap[`${endChapterNumber}`];
    return { startChapter, endChapter };
  }
})(module.exports);
