// ====  modals ====
const modalSingle = class {
    // map key val pair of all associated functions pertaining to modal behavior
    // i.e map contains a function

    addFunc(label, func) {
        this.functions.set(label, func);
    }

    getFunc(label) {
        return this.functions.get(label);
    }

    close() {
        this.modal.classList.add("hidden");
        this.overlay.classList.add("hidden");
    }

    open() {
        this.modal.focus(); // BUG broken
        this.modal.classList.remove("hidden");
        this.overlay.classList.remove("hidden");
    }

    constructor(modalName, userClosable) {
        this.functions = new Map();

        this.modal = document.querySelector("." + modalName);
        this.overlay = document.querySelector(".overlay");
        this.openBtn = document.querySelector(".btn-open");
        this.closeBtn = document.querySelector(".btn-close");

        if (userClosable) {
            this.overlay.addEventListener("click", this.constructor.close);
            this.openBtn.addEventListener("click", this.constructor.open);
            document.addEventListener("keydown", (e) => {
                if (
                    (e.key === "Enter" || e.key === "Escape") &&
                    !this.modal.classList.contains("hidden")
                ) {
                    this.close();
                }
            });
        }
    }
};

let modalIntro = new modalSingle("modalIntro", false);
modalIntro.addFunc("connectToTerminal", () => {
    connectClicked = true;
    modalIntro.close();
    term.write("Connecting... ");
    startTerm();
});
document.getElementById("connectToTerminal").onclick =
    modalIntro.getFunc("connectToTerminal");

let modalWarn = new modalSingle("modalWarn", true);

let modalLost = new modalSingle("modalLost", true);

let modalReconn = new modalSingle("modalReconn", true);

let modalReconnConfirmation = new modalSingle("modalReconnConfirmation", false);

// ====  xtermjs ====
const fitaddon = new FitAddon.FitAddon();
const termElement = document.querySelector("body");
const term = new Terminal({
    theme: {
        background: "#000000",
        cursor: "#ffffff",
    },
});
term.loadAddon(fitaddon);
term.open(termElement);
term.loadAddon(fitaddon);

fitaddon.fit();

// ==== program ====
function getClientIP() {
    let url =
        "https://ipgeolocation.abstractapi.com/v1/?api_key=3cf2f915e01c4d83a3b0e7b887e962d5";
    return postAsyncPromise("GET", url, null);
}

function postAsyncPromise(mode, url, toSend) {
    const xhr = new XMLHttpRequest();

    xhr.responseType = "json";
    xhr.open(mode, url, true);
    xhr.setRequestHeader("Content-Type", "application/json");

    // console.log(toSend)
    xhr.send(toSend);

    return new Promise((resolve, reject) => {
        xhr.onreadystatechange = () => {
            if (xhr.readyState === XMLHttpRequest.DONE && xhr.status === 200) {
                // console.log(xhr.response)
                resolve(xhr.response);
            } else if (
                xhr.readyState === XMLHttpRequest.DONE &&
                xhr.status === 429
            ) {
                reject("user refreshed page too quick");
            }
        };
    });
}

var socket;

function runTerm(address) {
    socket = io.connect(address);

    socket.on("output", (data) => {
        term.write(data);
    });

    term.onData((data) => {
        idleCount = 0;
        socket.emit("write", data);
    });

    term.onResize(function (evt) {
        console.log("term onresize");
        const terminal_size = {
            Width: evt.cols,
            Height: evt.rows,
        };
        socket.send("\x04" + JSON.stringify(terminal_size));
    });

    const xterm_resize_ob = new ResizeObserver(function (entries) {
        try {
            fitaddon && fitaddon.fit();
        } catch (err) {
            console.log(err);
        }
    });

    socket.emit("write", "\r");

    // start observing for resize
    xterm_resize_ob.observe(termElement);
}

// var idleMax = 10 * 60; // must be synced between containerPseudoTerminal.js
var idleMax = 2 * 60;
var idleCount = 0;
function timeout() {
    if (isTermRunning) {
        idleCount = idleCount + 1;

        document.getElementById("timeCounter").innerHTML = secToMin(
            idleMax - idleCount
        );

        // if (idleCount === 3 * 60) {
        if (idleCount === 20) {
            modalWarn.open();
        }

        if (idleCount === idleMax) {
            isTermRunning = false;
            modalLost.open();
        }
    }
}

function secToMin(secs) {
    let mins = Math.floor(secs / 60);
    let extraSecs = secs % 60;
    mins = mins < 10 ? "0" + mins : mins;
    extraSecs = extraSecs < 10 ? "0" + extraSecs : extraSecs;
    return mins + ":" + extraSecs;
}

function startTermWithAddress(clientIP) {
    let url = nodeServerAddr + "getPseudoTerminalAddress";
    let toSend = {
        IP: clientIP,
    };

    postAsyncPromise("POST", url, JSON.stringify(toSend)).then(
        (result) => {
            // console.log("result.podName")
            // console.log(result.podName)
            podName = result.podName;
            runTerm(result.ip);
        },

        (error) => {
            log.errror(error);
        }
    );
}

var clientIP;
var podName;
// var nodeServerAddr = "http://localhost:5252";
var nodeServerAddr = "http://64.227.106.28:31668/"; // TODO hardcoded
var isTermRunning = false;
var connectClicked = false;
//
var interval = setInterval(timeout, 1000);

function indexLoad() {
    if (!sessionStorage.tabID) {
        sessionStorage.tabID = genTabID();
        modalIntro.open();
    } else if (!connectClicked) {
        modalIntro.open();
    } else {
        startTerm();
        modalReconn.open();
    }
}

function genTabID() {
    return Math.random().toString();
}

indexLoad();

async function startTerm() {
    getClientIP().then(
        (result) => {
            isTermRunning = true;
            clientIP = result.ip_address;
            clientIP = sessionStorage.tabID;

            startTermWithAddress(clientIP);
        },

        async (error) => {
            console.log(error);
            console.log("Abstractapi call failed. trying again");
            await new Promise((r) => setTimeout(r, 1200));
            startTerm();
        }
    );
}
