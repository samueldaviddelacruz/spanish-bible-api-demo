((hasher)=>{


const crypto = require("crypto");
hasher.createSalt = () => {
const len = 8;

return crypto.randomBytes(Math.ceil(len/2)).toString('hex').substring(0,len);

};

hasher.computeHash = (source,salt)=>{
const hmac = crypto.createHmac("sha1",salt);
const hash = hmac.update(source);
return hash.digest("hex");
}


})(module.exports)