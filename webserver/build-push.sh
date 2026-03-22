#!/bin/sh
set -e

docker build -t wt-webserver .

docker tag wt-webserver registry.digitalocean.com/mdr/wt-webserver
docker push registry.digitalocean.com/mdr/wt-webserver
