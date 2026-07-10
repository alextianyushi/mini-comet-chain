#!/usr/bin/env bash

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMETBFT_VERSION="v0.39.0"
DATA_DIR="${DATA_DIR:-$PROJECT_DIR/data}"
COMETBFT_HOME="${COMETBFT_HOME:-$DATA_DIR/cometbft}"
KV_HOME="${KV_HOME:-$DATA_DIR/kvstore}"
SOCKET_PATH="${SOCKET_PATH:-$DATA_DIR/mini-comet-chain.sock}"
APP_PID=""
COMET_PID=""

cleanup() {
  trap - EXIT INT TERM

  if [[ -n "$COMET_PID" ]] && kill -0 "$COMET_PID" 2>/dev/null; then
    kill -INT "$COMET_PID" 2>/dev/null || true
    wait "$COMET_PID" 2>/dev/null || true
  fi

  if [[ -n "$APP_PID" ]] && kill -0 "$APP_PID" 2>/dev/null; then
    kill -INT "$APP_PID" 2>/dev/null || true
    wait "$APP_PID" 2>/dev/null || true
  fi

  rm -f "$SOCKET_PATH"
}

trap cleanup EXIT INT TERM

cd "$PROJECT_DIR"
mkdir -p "$DATA_DIR"

echo "Building mini-comet-chain..."
go build -mod=mod -o mini-comet-chain .

if [[ ! -f "$COMETBFT_HOME/config/genesis.json" ]]; then
  echo "Initializing CometBFT home at $COMETBFT_HOME..."
  go run "github.com/cometbft/cometbft/cmd/cometbft@$COMETBFT_VERSION" \
    init --home "$COMETBFT_HOME"
fi

# A socket left behind by an unclean shutdown cannot be reused.
rm -f "$SOCKET_PATH"

echo "Starting KV store..."
./mini-comet-chain \
  -kv-home "$KV_HOME" \
  -socket-addr "unix://$SOCKET_PATH" &
APP_PID=$!

# Wait briefly for the ABCI server to create its socket before starting CometBFT.
for _ in {1..100}; do
  if [[ -S "$SOCKET_PATH" ]]; then
    break
  fi

  if ! kill -0 "$APP_PID" 2>/dev/null; then
    echo "KV store stopped before creating its socket." >&2
    wait "$APP_PID"
  fi

  sleep 0.1
done

if [[ ! -S "$SOCKET_PATH" ]]; then
  echo "Timed out waiting for Unix socket: $SOCKET_PATH" >&2
  exit 1
fi

echo "Starting CometBFT... (press Ctrl+C to stop both processes)"
go run "github.com/cometbft/cometbft/cmd/cometbft@$COMETBFT_VERSION" \
  start \
  --home "$COMETBFT_HOME" \
  --proxy_app "unix://$SOCKET_PATH" &
COMET_PID=$!

wait "$COMET_PID"
