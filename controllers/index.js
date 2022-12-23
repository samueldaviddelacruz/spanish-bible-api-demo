( (controllers) =>{
  
    const bibleController = require("./bibleController")
    controllers.init =  (app) =>{
        
        bibleController.init(app);
    }

})(module.exports);