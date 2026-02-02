#!/bin/bash
# Script para gerar código Go a partir dos arquivos .proto

set -e

PROTO_DIR="api/proto/v1"
PB_OUT_DIR="pkg/pb/v1"

echo "Creating output directory..."
mkdir -p $PB_OUT_DIR

echo "Generating protobuf code..."
protoc \
    --proto_path=$PROTO_DIR \
    --go_out=$PB_OUT_DIR --go_opt=paths=source_relative \
    --go-grpc_out=$PB_OUT_DIR --go-grpc_opt=paths=source_relative \
    $PROTO_DIR/*.proto

echo "Done! Generated files in $PB_OUT_DIR"
ls -la $PB_OUT_DIR
