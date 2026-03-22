#!/bin/sh
set -e

docker build -t man-pty . -f $1

docker tag man-pty registry.digitalocean.com/mdr/man-pty
docker push registry.digitalocean.com/mdr/man-pty
