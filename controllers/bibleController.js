(function (bibleController) {
    //requires 'index inside of data folder'
    const ApiRequestHandler = require("../Request Handlers/API.Handler.js")

   // const request = require("request");
    bibleController.init =  (app) =>{

        app.get("/api/books",ApiRequestHandler.onGetApiBooks);
        app.get("/api/books/:bookId",ApiRequestHandler.onGetApiBook);
        
        app.get(`/api/books/:bookId/verses/:verseRange(\\d+)`,ApiRequestHandler.onGetVersesByChapterNumber); //done
        
        app.get(`/api/books/:bookId/verses/:verseRange(\\d+:\\d+)`,ApiRequestHandler.onGetVerseByChapterAndVerseNumber); //done

        app.get(`/api/books/:bookId/verses/:verseRange(\\d+-\\d+:\\d+)`,ApiRequestHandler.onGetVersesFromChapterUntilVerse);
    
        app.get(`/api/books/:bookId/verses/:verseRange(\\d+:\\d+-\\d+:\\d+)`,ApiRequestHandler.onGetVersesFromVerseToVerse);
        
        app.get(`/api/books/:bookId/verses/:verseRange(\\d+-\\d+)`,ApiRequestHandler.onGetVersesFromChapterToChapter);

    }


})(module.exports);