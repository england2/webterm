## Project Explanation

Webterm is a microservice-style application that provides a web interface to access Linux containers hosted in a Kubernetes cluster.

Technologies involved are Go, Kubernetes, ArgoCD, Docker, Helm, Node.js, and JavaScript.

### Technical Overview

- Webterm has three deployable services: `webserver`, `pseudo-terminal-manager`, and `pseudo-terminal`
- We use a Go program for cluster orchestration and terminal allocation, and Node.js as a terminal backend system
- The website hosts a static bundle serving a terminal UI based on `xterm.js`
- Deployed using a Helm chart, with image tags updated by GitHub Actions in [`pipeline.yml`](https://github.com/england2/webterm/blob/main/.github/workflows/pipeline.yml)
  - (See [shellbin](https://github.com/england2/shellbin) for an explanation of a similar pipeline.)
- Terminal pods use `node-pty` to bind a real shell process to a browser socket connection; keyboard inputs and results are sent over the wire

See the full project writeup at [etengland.me/webterm](https://etengland.me/webterm).

