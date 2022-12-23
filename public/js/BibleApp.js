(function() {
  const app = angular.module("bibleApp", []);

  const bibleController = function($http, $sce, $q, $timeout, $scope) {
    //console.log('angular ready to rumble');
    //sa
    let bible = this;
    bible.IsLoadingVerses = false;
    bible.IsOnFullSearch = false;
   
    (async () => {
      bible.renderHtml = function(htmlCode) {
        return $sce.trustAsHtml(htmlCode);
      };
      bible.ListVerses = async chapter => {
        if(bible.selectedChapter) {
            bible.selectedChapter.verses = await getVerses(bible.selectedChapter.id);
            bible.IsLoadingVerses = false;
            $scope.$apply();
        }
        //console.log(bible.selectedChapter)
        //bible.selectedChapter.verses = [];
        _resetWordCountVars();
        
       
      };
      //Dios

      const getVerses = async chapterId => {
        bible.IsLoadingVerses = true;
       
        let result = await $http.get(`/api/verses/${chapterId}`);

        return result.data;
      };

      const _resetWordCountVars = function() {
        bible.wordCount = 0;
        bible.foundWords = null;
      };

      bible.searchWholeBook = async () => {
        if (bible.searchTextInBook) {
          bible.selectedChapter = {};
          bible.selectedChapter.verses = [];

          bible.IsOnFullSearch = true;
          _resetWordCountVars();

          let chapters = bible.selectedBook.chapters;
          for (let chapter of chapters) {
            let verses = await getVerses(chapter.id);

            addAndUpdateSearchedVerses(verses);
          }

          bible.foundWords =
            "La palabra " +
            bible.searchTextInBook +
            " Se encontro " +
            bible.wordCount +
            " Veces";
          bible.IsLoadingVerses = false;
          bible.IsOnFullSearch = false;

          $scope.$apply();
        }
      };
      const addAndUpdateSearchedVerses = verses => {
        for (let verse of verses) {
          if (
            verse.text
              .toUpperCase()
              .includes(bible.searchTextInBook.toUpperCase())
          ) {
            bible.wordCount++;
            verse.text = verse.text.replace(
              bible.searchTextInBook,
              "<b>" + bible.searchTextInBook.toUpperCase() + "</b>"
            );
            bible.selectedChapter.verses.push(verse);
          }
        }
      };

      bible.addToFavoriteVerses = async function(verse) {
        await addToFavorite(verse, $http);
        bible.VerseAdded = true;
        $scope.$apply();
        $timeout(() => {
          bible.VerseAdded = false;
          $scope.$apply();
        }, 3000);
      };

      bible.books = await getBooks($http);
      bible.selectedBook = bible.books[0];
      $scope.$apply();
    })();
  };
  const addToFavorite = async (verse, $http) => {
    return await $http.post("/api/addToFavorite", {
      verseId: verse.id,
      reference: verse.reference
    });
  };

  const getBooks = async $http => {
    let result = await $http.get("/api/books");

    return result.data;
  };

  const accountController = function($http, $sce) {
    let account = this;

    account.renderHtml = function(htmlCode) {
      return $sce.trustAsHtml(htmlCode);
    };
    function getMyVerses() {
      $http.get("/api/getMyVerses").then(function(result) {
        
        account.MyFavoriteVerses = [...result.data];

      });
    }

    account.removeFromFavorites = function(verse) {
      $http
        .post("/api/removeFromFavorites", {
          verseId: verse.id,
          reference: verse.reference
        })
        .then(function() {
          getMyVerses();
        });
    };

    getMyVerses();
  };

  app.controller("bibleController", bibleController);

  app.controller("accountController", accountController);
})();
