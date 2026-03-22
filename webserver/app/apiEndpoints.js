const needle = require("needle");

const defaultPseudoTerminalManagerAddr =
    "http://webterm-manager:6262";
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
                return;
            }

            if (err) {
                console.log(err);
                reject(err);
                return;
            }
        });
    });
}

function getRequestProtocol(req) {
    return (req.get("x-forwarded-proto") || req.protocol || "http")
        .split(",")[0]
        .trim();
}

function extractPort(address) {
    if (!address) {
        return "";
    }

    try {
        return new URL(address).port;
    } catch (error) {
        const match = address.match(/:(\d+)$/);
        return match ? match[1] : "";
    }
}

function buildBrowserAddress(req, rawAddress) {
    const port = extractPort(rawAddress);
    if (!port) {
        return rawAddress;
    }

    return `${getRequestProtocol(req)}://${req.hostname}:${port}`;
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
    const clientIP = req.body.IP;

    let addr = pseudoTerminalManagerAddr + "/getPseudoTerminalAddress";
    let reqBody = { ip: clientIP };
    let addrPromise = needlePostPromise(addr, reqBody);

    console.log("- addrPromise -\n", addrPromise); //t

    addrPromise.then(
        (result) => {
            console.log("- result -\n", result); //t
            let eq = "========================"; //t
            console.log(`${eq}${eq}${eq}${eq}`); //t

            result.ip = buildBrowserAddress(req, result.ip);
            res.send(result);
        },
        (error) => {
            console.error(error);
            res.status(502).send({ error: "failed to get pseudo-terminal address" });
        }
    );
}

module.exports = {
    getPseudoTerminalAddress,
    pseudoTerminalManagerAddr,
};
