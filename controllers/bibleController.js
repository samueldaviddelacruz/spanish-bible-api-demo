(function (bibleController) {
    //requires 'index inside of data folder'
    const ApiRequestHandler = require("../Request Handlers/API.Handler.js")
    const ViewsRequestHandler = require("../Request Handlers/Views.Handler")
   
    const auth = require("../auth");
   // const request = require("request");
    bibleController.init =  (app) =>{

        app.get("/api/books",ApiRequestHandler.onGetApiBooks);
        app.get("/api/books/:bookId",ApiRequestHandler.onGetApiBook);
        
        app.get(`/api/books/:bookId/verses/:verseRange(\\d+)`,ApiRequestHandler.onGetVersesByChapterNumber);
        
        app.get(`/api/books/:bookId/verses/:verseRange(\\d+:\\d+)`,ApiRequestHandler.onGetVerseByChapterAndVerseNumber); 

        app.get(`/api/books/:bookId/verses/:verseRange(\\d+-\\d+:\\d+)`,ApiRequestHandler.onGetVersesFromChapterUntilVerse);
        
        app.get(`/api/books/:bookId/verses/:verseRange(\\d+:\\d+-\\d+)`,ApiRequestHandler.onGetVersesFromVerseUntilChapter);

        app.get(`/api/books/:bookId/verses/:verseRange(\\d+:\\d+-\\d+:\\d+)`,ApiRequestHandler.onGetVersesFromVerseToVerse);
        
        app.get(`/api/books/:bookId/verses/:verseRange(\\d+-\\d+)`,ApiRequestHandler.onGetVersesFromChapterToChapter);

        app.get("/api/verses/:chapterId", ApiRequestHandler.onGetApiVersesByChapterId);
        app.get("/api/verses/:verseId", ApiRequestHandler.onGetApiSingleVerse);


        app.get("/api/getSingleVerse/:verseId", ApiRequestHandler.onGetApiSingleVerse);
        


        app.get("/api/getMyVerses", auth.ensureAuthenticated,ApiRequestHandler.onGetMyFavoriteVerses);

        app.post("/api/addToFavorite", auth.ensureAuthenticated, ApiRequestHandler.onAddToFavorites);

        app.post("/api/removeFromFavorites", auth.ensureAuthenticated, ApiRequestHandler.onRemoveFromFavorites);



        app.get("/bible", auth.ensureAuthenticated,ViewsRequestHandler.onGetBibleView);

        app.get("/FavoriteVerses", auth.ensureAuthenticated,ViewsRequestHandler.onGetMyFavoriteVersesView);

        
    }


})(module.exports);