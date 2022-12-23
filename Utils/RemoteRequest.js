/**
 * Created by Samuel on 7/4/2016.
 */
const request = require("request-promise");

(remoteRequestHandler => {
  remoteRequestHandler.createRequest = async function(url) {
    try {
      const requestOptions = {
        url: url,
        json: true
      };
      return await request(requestOptions);
    } catch (ex) {
      console.log(err);
    }
    return response;
  };
})(module.exports);
