const needle = require("needle");
const os = require("os");
const pty = require("node-pty");
const socketio = require("socket.io");

const PORT = 7070 || process.env.PORT;
const socket = new socketio.Server(PORT, {
    cors: {
        origin: "*",
    },
});

socket.on("connection", (sock) => {
    onFirstConnect();

    sock.on("write", (data) => {
        timeCount = 0;
        ptyProcess.write(data);
    });
});

shell = os.platform() === "win32" ? "powershell.exe" : "fish";

var ptyProcess = pty.spawn(shell, [], {
    name: "xterm-color",
    cols: 80,
    rows: 30,
    cwd: process.env.HOME,
    env: process.env,
});

ptyProcess.on("data", function (data) {
    process.stdout.write(data);
    socket.emit("output", data);
});

var is_first_connection = true;

function onFirstConnect() {
    if (is_first_connection) {
        is_first_connection = false;
        var interval = setInterval(timeout, 1000);
        ptyProcess.write("fish\r");
        ptyProcess.write("cat /app/welcome.txt\r");
        // rm -rf /app
    }
}

var timeCount = 0;
function timeout() {
    timeCount += 1;
    if (timeCount === 10 * 60) {
        killThisPod();
    }
}

const defaultPtyManAddr = "http://man-pty:6262";
const ptyManAddr = (process.env.PTY_MANAGER_ADDR || defaultPtyManAddr).replace(
    /\/+$/,
    ""
);
const podName = process.env.HOSTNAME;

function killThisPod() {
    let toSend = {
        IP: "NA",
        PODNAME: podName,
    };

    let addr = ptyManAddr + "/killUserPod";

    let podKillPromise = needlePostPromise(addr, toSend);
    console.log("- podKillPromise -\n", podKillPromise); //t
    podKillPromise.then(
        (result) => {
            console.log("- result -\n", result); //t
            console.log("======================"); //t
            // res.send(result);
        },
        (error) => {
            console.error(error);
        }
    );
}

function needlePostPromise(addr, reqBody) {
    return new Promise((resolve, reject) => {
        needle.request("post", addr, reqBody, { json: true }, (err, resp) => {
            if (!err) {
                console.log("- response -\n", resp.body); //t
                resolve(resp.body);
            }

            if (err) {
                console.log(err);
                reject(err);
            }

            reject(new Error("reached end"));
        });
    });
}
