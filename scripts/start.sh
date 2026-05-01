#!/bin/bash
cd "$(dirname "$0")/.."
go build -o bin/short-url-service cmd/main.go
./bin/short-url-service