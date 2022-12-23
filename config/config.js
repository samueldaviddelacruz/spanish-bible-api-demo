
const getConfig = () => {
  if (process.env.MONGO_URL && process.env.DB_NAME) {
    return {
      mongoDatabase: {
        mongoUrl: process.env.MONGO_URL,
        dbName: process.env.DB_NAME
      }
    };
  }
  try {
    const keys = require('./keys')
    return keys
  }catch(err) {
    throw err
  }
};

module.exports = getConfig()
