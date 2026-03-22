# compile pseudo-terminal-manager
FROM golang:1.19.8

WORKDIR /src
COPY pseudo-terminal-manager-api/* ./
# RUN go mod tidy 
RUN CGO_ENABLED=0 go build -a -installsuffix cgo -o pseudo-terminal-manager .

# build pod container
# TODO test alpine
FROM debian
EXPOSE 6262

WORKDIR /app
COPY start.sh .
COPY --from=0 /src/pseudo-terminal-manager .

CMD ["sh", "start.sh"]
