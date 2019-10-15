#!/usr/bin/env bash

set -o errexit

env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o broker-proxy .

docker build -t broker-proxy -f ./deploy/Dockerfile .
docker tag broker-proxy gcr.io/hazel-field-195020/broker-proxy
docker push gcr.io/hazel-field-195020/broker-proxy

rm -f broker-proxy
