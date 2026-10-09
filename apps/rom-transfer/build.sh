#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"

# Server (Linux, runs on the NixOS homeserver)
CGO_ENABLED=0 go build -o bin/server ./cmd/server

# Agent — Linux (BC-250 and other Linux clients)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/rom-agent-linux ./cmd/agent

# Agent — Windows 10
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/rom-agent.exe ./cmd/agent

echo "Built: bin/server  bin/rom-agent-linux  bin/rom-agent.exe"
