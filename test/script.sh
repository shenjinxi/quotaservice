#!/bin/bash

set -ex

case "$TEST" in
  "UI")
    cd admin/public/
    nvm install 6
    npm install
    npm run lint
    npm run dist
    ;;
  "COMPILE_PROTOS")
    wget https://github.com/google/protobuf/releases/download/v3.0.0/protoc-3.0.0-linux-x86_64.zip
    unzip -d protoc protoc-3.0.0-linux-x86_64.zip

    go get -u github.com/golang/protobuf/protoc-gen-go

    export PATH="$PATH:./protoc/bin/"

    # Make sure protos haven't changed + test compile_protos.sh
    bin/compile_protos.sh && git diff --exit-code -- protos/
    ;;
  *)
    go test -race -v ./...
    ;;
esac
