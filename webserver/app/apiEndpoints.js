const needle = require("needle");

const defaultPseudoTerminalManagerAddr =
    "http://pseudo-terminal-manager:6262";
const pseudoTerminalManagerAddr = (
    process.env.PSEUDO_TERMINAL_MANAGER_ADDR ||
    process.env.PTY_MANAGER_ADDR ||
    defaultPseudoTerminalManagerAddr
).replace(
    /\/+$/,
    ""
);

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

function killUserPod(req, res) {
    console.log("in killUserPod");
    console.log("- request -\n", req.body); //t

    let addr = pseudoTerminalManagerAddr + "/killUserPod";

    let podKillPromise = needlePostPromise(addr, req.body);

    console.log("- addrPromise -\n", podKillPromise); //t

    podKillPromise.then(
        (result) => {
            console.log("- result -\n", result); //t
            let eq = "========================"; //t
            console.log(`${eq}${eq}${eq}${eq}`); //t

            res.send(result);
        },
        (error) => {
            console.error(error);
        }
    );
}

function getPseudoTerminalAddress(req, res) {
    console.log("in getPseudoTerminalAddress");
    console.log("- request -\n", req.body); //t
    clientIP = req.body.IP;

    let addr = pseudoTerminalManagerAddr + "/getPseudoTerminalAddress";
    let reqBody = { ip: clientIP };
    let addrPromise = needlePostPromise(addr, reqBody);

    console.log("- addrPromise -\n", addrPromise); //t

    addrPromise.then(
        (result) => {
            console.log("- result -\n", result); //t
            let eq = "========================"; //t
            console.log(`${eq}${eq}${eq}${eq}`); //t

            res.send(result);
        },
        (error) => {
            console.error(error);
        }
    );
}

module.exports = {
    getPseudoTerminalAddress,
    pseudoTerminalManagerAddr,
};
