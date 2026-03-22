// setup
const endpoints = require("./apiEndpoints");
const bodyParser = require("body-parser");
const express = require("express");

const app = express();
app.use(bodyParser.text());
app.use(bodyParser.json());
app.use(bodyParser.urlencoded({ extended: false }));
app.use((req, res, next) => {
    res.setHeader(
        "Access-Control-Allow-Origin",
        process.env.CORS_ALLOW_ORIGIN || "*"
    );
    res.header(
        "Access-Control-Allow-Headers",
        "Origin, X-Requested-With, Content-Type, Accept"
    );
    next();
});

//app
function main() {
    app.post(
        "/getPseudoTerminalAddress",
        endpoints.getPseudoTerminalAddress
    );

    app.use(express.static("public"));

    let listenPort = "5252";
    if (process.env.LISTENPORT) {
        listenPort = process.env.LISTENPORT;
    }

    app.listen(listenPort, () => {
        console.log("running on port %d", listenPort);
    });
}

main();
