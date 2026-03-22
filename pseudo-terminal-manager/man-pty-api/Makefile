default:
	CGO_ENABLED=0 go build -a -installsuffix cgo -o man-pty .

push:
	kubectl cp man-pty ptys/${WORKPOD}:/app
