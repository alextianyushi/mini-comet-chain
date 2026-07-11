#!/usr/bin/env bash

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="$PROJECT_DIR/data"
COMETBFT_BIN="$PROJECT_DIR/bin/cometbft"
APP_PIDS=()
COMET_PIDS=()

p2p_ports=(26656 26756 26856 26956)
rpc_ports=(26657 26757 26857 26957)

cleanup() {
  trap - EXIT INT TERM

  echo
  echo "Stopping the four-node network..."
  for pid in "${COMET_PIDS[@]}"; do
    kill -INT "$pid" 2>/dev/null || true
  done
  for pid in "${COMET_PIDS[@]}"; do
    wait "$pid" 2>/dev/null || true
  done
  for pid in "${APP_PIDS[@]}"; do
    kill -INT "$pid" 2>/dev/null || true
  done
  for pid in "${APP_PIDS[@]}"; do
    wait "$pid" 2>/dev/null || true
  done
  for node in 1 2 3 4; do
    rm -f "$DATA_DIR/node$node/abci.sock"
  done
}

trap cleanup EXIT INT TERM

cd "$PROJECT_DIR"

if [[ ! -x "$COMETBFT_BIN" ]]; then
  echo "CometBFT is not installed locally. Run ./setup-four-nodes.sh first." >&2
  exit 1
fi

for node in 1 2 3 4; do
  if [[ ! -f "$DATA_DIR/node$node/cometbft/config/genesis.json" ]]; then
    echo "Node $node is not initialized. Run ./setup-four-nodes.sh first." >&2
    exit 1
  fi
done

echo "Building mini-comet-chain..."
go build -mod=mod -o mini-comet-chain .

node_ids=()
for node in 1 2 3 4; do
  node_ids+=("$("$COMETBFT_BIN" show-node-id --home "$DATA_DIR/node$node/cometbft")")
done

for node in 1 2 3 4; do
  socket="$DATA_DIR/node$node/abci.sock"
  rm -f "$socket"
  echo "Starting KV application $node..."
  ./mini-comet-chain \
    -kv-home "$DATA_DIR/node$node/kvstore" \
    -socket-addr "unix://$socket" \
    >"$DATA_DIR/node$node/app.log" 2>&1 &
  APP_PIDS+=("$!")
done

for node in 1 2 3 4; do
  socket="$DATA_DIR/node$node/abci.sock"
  for _ in {1..100}; do
    [[ -S "$socket" ]] && break
    sleep 0.1
  done
  if [[ ! -S "$socket" ]]; then
    echo "KV application $node did not create its socket." >&2
    exit 1
  fi
done

for node in 1 2 3 4; do
  index=$((node - 1))
  peers=""
  for peer in 1 2 3 4; do
    [[ "$peer" -eq "$node" ]] && continue
    peer_index=$((peer - 1))
    entry="${node_ids[$peer_index]}@127.0.0.1:${p2p_ports[$peer_index]}"
    [[ -n "$peers" ]] && peers+=","
    peers+="$entry"
  done

  echo "Starting CometBFT node $node (RPC :${rpc_ports[$index]}, P2P :${p2p_ports[$index]})..."
  "$COMETBFT_BIN" start \
    --home "$DATA_DIR/node$node/cometbft" \
    --moniker "node$node" \
    --proxy_app "unix://$DATA_DIR/node$node/abci.sock" \
    --p2p.laddr "tcp://127.0.0.1:${p2p_ports[$index]}" \
    --p2p.external-address "127.0.0.1:${p2p_ports[$index]}" \
    --p2p.persistent_peers "$peers" \
    --rpc.laddr "tcp://127.0.0.1:${rpc_ports[$index]}" \
    >"$DATA_DIR/node$node/cometbft.log" 2>&1 &
  COMET_PIDS+=("$!")
done

echo
echo "Four-node network is running."
echo "RPC endpoints:"
for node in 1 2 3 4; do
  index=$((node - 1))
  echo "  node$node: http://127.0.0.1:${rpc_ports[$index]}"
done
echo "Logs are stored under data/node*/. Press Control+C to stop all nodes."

while true; do
  alive=0
  for pid in "${APP_PIDS[@]}" "${COMET_PIDS[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      alive=$((alive + 1))
    fi
  done
  if [[ "$alive" -eq 0 ]]; then
    echo "All network processes have stopped." >&2
    exit 1
  fi
  sleep 1
done
