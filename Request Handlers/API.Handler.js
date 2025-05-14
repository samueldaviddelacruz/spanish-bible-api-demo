/**
 * Created by Samuel on 7/3/2016.
 */

const DB = require("../data");

(ApiRequestHandler => {
  ApiRequestHandler.onGetApiBooks = async (nodeRequest, nodeResponse) => {
    const response = await DB.getBibleBooks();
    sendResponse(nodeResponse, response);
  };

  ApiRequestHandler.onGetApiBook = async (nodeRequest, nodeResponse) => {
    const response = await DB.getBibleBookById(nodeRequest.params.bookId);
    sendResponse(nodeResponse, response);
  };
  ApiRequestHandler.onGetVersesByChapterNumber = async (
    nodeRequest,
    nodeResponse
  ) => {
    try {
      const response = await DB.getVersesByChapterNumber(
        nodeRequest.params.bookId,
        nodeRequest.params.verseRange
      );
      sendResponse(nodeResponse, response);
    } catch (error) {
      nodeResponse.send(500, error);
    }
  };
  ApiRequestHandler.onGetVerseByChapterAndVerseNumber = async (
    nodeRequest,
    nodeResponse
  ) => {
    try {
      const response = await DB.getVerseByChapterAndVerseNumber(
        nodeRequest.params.bookId,
        nodeRequest.params.verseRange
      );
      sendResponse(nodeResponse, response);
    } catch (error) {
      nodeResponse.send(500, error);
    }
  };
  ApiRequestHandler.onGetVersesFromVerseUntilChapter = async (
    nodeRequest,
    nodeResponse
  ) => {
    try {
      const response = await DB.getVersesFromVerseUntilChapter(
        nodeRequest.params.bookId,
        nodeRequest.params.verseRange
      );
      sendResponse(nodeResponse, response);
    } catch (error) {
      nodeResponse.send(500, error);
    }
  };

  ApiRequestHandler.onGetVersesFromChapterUntilVerse = async (
    nodeRequest,
    nodeResponse
  ) => {
    try {
      const response = await DB.getVersesFromChapterUntilVerse(
        nodeRequest.params.bookId,
        nodeRequest.params.verseRange
      );
      sendResponse(nodeResponse, response);
    } catch (error) {
      nodeResponse.send(500, error);
    }
  };


  ApiRequestHandler.onGetVersesFromVerseToVerse = async (
    nodeRequest,
    nodeResponse
  ) => {
    try {
      const response = await DB.getVersesFromVerseToVerse(
        nodeRequest.params.bookId,
        nodeRequest.params.verseRange
      );
      sendResponse(nodeResponse, response);
    } catch (error) {
      nodeResponse.send(500, error);
    }
  };

  ApiRequestHandler.onGetVersesFromChapterToChapter = async (
    nodeRequest,
    nodeResponse
  ) => {
    try {
      const response = await DB.getVersesFromChapterToChapter(
        nodeRequest.params.bookId,
        nodeRequest.params.verseRange
      );
      sendResponse(nodeResponse, response);
    } catch (error) {
      nodeResponse.send(500, error);
    }
  }; 


 ApiRequestHandler.addChapterNumberAndVerseNumber = async (
    nodeRequest,
    nodeResponse
  ) => {
    try {
      const response = await DB.addChapterNumberAndVerseNumber(
        nodeRequest.params.bookId,
        nodeRequest.params.verseRange
      );
      sendResponse(nodeResponse, response);
    } catch (error) {
      nodeResponse.send(500, error);
    }
  }

  function sendResponse(nodeResponse, content) {
    nodeResponse.set("Content-Type", "application/json");
    nodeResponse.send(content);
  }

})(module.exports);
