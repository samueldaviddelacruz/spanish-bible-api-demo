//'auth/index.js'
(function (auth) {
  const data = require("../data");
  const hasher = require("./hasher");
  const passport = require("passport");
  const ViewsRequestHandler = require("../Request Handlers/Views.Handler");
  const localStrategy = require("passport-local").Strategy;

  async function userVerify(username, password, next) {
    try {
      const user = await data.getUser(username.toUpperCase());

      const testHash = hasher.computeHash(password, user.salt);

      if (testHash === user.passwordHash) {
        next(null, user);
      } else {
        next("Invalid Credentials", false, {
          message: "Invalid Credentials",
        });
      }
    } catch (err) {
      next(null, false, {
        message: "User Doesnt Exists",
      });
    }
  }
  auth.ensureAuthenticated = (req, res, next) => {
    if (req.isAuthenticated()) {
      next();
    } else {
      res.redirect("/");
    }
  };

  auth.ensureApiAuthenticated = (req, res, next) => {
    if (req.isAuthenticated()) {
      next();
    } else {
      res.send(401, "Not authorized");
    }
  };

  auth.init = (app) => {
    //setup passport authentication
    passport.use(new localStrategy(userVerify));
    passport.serializeUser((user, next) => {
      try {
        next(null, user.username);
      } catch (err) {
        console.log(err);
      }
    });
    passport.deserializeUser(async (key, next) => {
      try {
        const user = await data.getUser(key);
        next(null, user);
      } catch (err) {
        next(null, false, {
          message: "failed to retrieve user",
        });
      }
    });

    app.use(passport.initialize());
    app.use(passport.session());
    app.get("/register", ViewsRequestHandler.onRegister);

    app.get("/", ViewsRequestHandler.onLogIn);

    app.post("/register", async (req, res, next) => {
      const newUser = getNewUserData(req);

      try {
        const user = await data.getUser(newUser.username);
        if (user) {
          req.flash("registrationError", user.username + "  ya esta en uso");
          res.redirect("/register");
        } else {
          await data.addUser(newUser);
          const authFunction = getAuthFunction(req, res);
          authFunction(req, res);
        }
      } catch (err) {
        console.log(err);
        req.flash("registrationError", "Could Not save User to Database");
        res.redirect("/register");
      }
    });
    const getNewUserData = (req) => {
      const salt = hasher.createSalt();
      return {
        name: req.body.name,
        email: req.body.email,
        username: req.body.username.toUpperCase(),
        passwordHash: hasher.computeHash(req.body.password, salt),
        salt: salt,
      };
    };

    app.get("/logout", ViewsRequestHandler.onLogOut);

    app.post("/login", (req, res, next) => {
      const authFunction = getAuthFunction(req, res);
      authFunction(req, res, next);
    });

    const getAuthFunction = (req, res) => {
      return passport.authenticate("local", getOnAuthCallback(req, res));
    };
    const getOnAuthCallback = (req, res) => {
      return ViewsRequestHandler.getOnLoginHandler(req, res);
    };
  };
})(module.exports);
