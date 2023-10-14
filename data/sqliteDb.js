const db = require('better-sqlite3')('Bible.db');
db.pragma('journal_mode = WAL');
module.exports = {
    sqliteDb: db
}
