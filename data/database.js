(database => {
  const MongoClient = require("mongodb").MongoClient;
  const { mongoUrl, dbName } = require("../config/config").mongoDatabase;
  const promise = require("bluebird");
  const getMongoDbConnection = async uri => {
    const promise = new Promise((resolve, reject) => {
      MongoClient.connect(uri, { useNewUrlParser: true }, function(
        err,
        client
      ) {
        if (err) {
          reject(err);
        }
        resolve(client);
      });
    });
    return promise;
  };
  let theDb = null;
  database.getDb = async () => {
    if (!theDb) {
      //connecto the database
      try {
        let client = await getMongoDbConnection(mongoUrl);

        theDb = {
          db: client.db(dbName),
          usuarios: client.db(dbName).collection("bible-usuarios"),
          bibleVerses: client.db(dbName).collection("bible_verses"),
          bibleBooks: client.db(dbName).collection("bible_books")
        };
        theDb.usuarios = promise.promisifyAll(theDb.usuarios);
        theDb.bibleVerses = promise.promisifyAll(theDb.bibleVerses);
        theDb.bibleBooks = promise.promisifyAll(theDb.bibleBooks);
        return theDb;
      } catch (err) {
        //next(err, null);
        throw err;
      }
    }
    return theDb;
  };
})(module.exports);
