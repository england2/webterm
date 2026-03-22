#!/bin/sh

if [ $# -eq 0 ]
then
    echo "missing container name arg"
    exit 1
fi

docker build -t $1 .
docker tag $1 registry.digitalocean.com/mdr/$1 
docker push registry.digitalocean.com/mdr/$1 

