default:
	CGO_ENABLED=0 go build -a -installsuffix cgo -o pseudo-terminal-manager .

push:
	kubectl cp pseudo-terminal-manager pseudo-terminals/${WORKPOD}:/app
