#!/usr/bin/env bash

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="$PROJECT_DIR/data"
GENERATED_DIR="$DATA_DIR/generated"
COMETBFT_VERSION="v0.39.0"
COMETBFT_BIN="$PROJECT_DIR/bin/cometbft"

cd "$PROJECT_DIR"

if [[ -e "$DATA_DIR" ]]; then
  echo "The data directory already exists." >&2
  echo "Remove it first if you want to create a new four-node network:" >&2
  echo "  rm -rf '$DATA_DIR'" >&2
  exit 1
fi

echo "Building the KV application and CometBFT..."
go build -mod=mod -o mini-comet-chain .
mkdir -p "$PROJECT_DIR/bin"
GOBIN="$PROJECT_DIR/bin" go install "github.com/cometbft/cometbft/cmd/cometbft@$COMETBFT_VERSION"

echo "Generating four validators with one shared genesis file..."
"$COMETBFT_BIN" testnet \
  --v 4 \
  --n 0 \
  --o "$GENERATED_DIR" \
  --populate-persistent-peers=false

for index in 0 1 2 3; do
  node=$((index + 1))
  mkdir -p "$DATA_DIR/node$node"
  mv "$GENERATED_DIR/node$index" "$DATA_DIR/node$node/cometbft"
  mkdir -p "$DATA_DIR/node$node/kvstore"
done

rmdir "$GENERATED_DIR"

echo
echo "Four-node network created under $DATA_DIR."
echo "Start it with:"
echo "  ./start-four-nodes.sh"
