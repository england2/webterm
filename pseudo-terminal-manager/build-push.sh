#!/bin/sh
set -e

docker build -t pseudo-terminal-manager . -f $1

docker tag pseudo-terminal-manager registry.digitalocean.com/mdr/pseudo-terminal-manager
docker push registry.digitalocean.com/mdr/pseudo-terminal-manager
