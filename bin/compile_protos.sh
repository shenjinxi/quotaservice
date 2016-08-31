#!/bin/sh

# Licensed under the Apache License, Version 2.0
# Details: https://raw.githubusercontent.com/maniksurtani/quotaservice/master/LICENSE

set -ex

protoc --go_out=plugins=grpc:. ./protos/*.proto --proto_path ./
protoc --go_out=plugins=grpc:. ./protos/config/*.proto --proto_path ./
sed -i '' -e 's/\(json:"\([^,]*\),omitempty"\)/\1 yaml:"\2"/g' ./protos/config/configs.pb.go

echo "Protos compiled. If you made any changes to protos/config/configs.proto, then please read protos/config/README.md now."
