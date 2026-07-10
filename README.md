# Mini Comet Chain

A KV store blockchain built with Go, CometBFT v0.39.0, ABCI, and BadgerDB.

## Components

- **CometBFT** orders transactions, produces blocks, and runs consensus.
- **KV application** validates and executes `key=value` transactions.
- **ABCI** connects CometBFT to the KV application.
- **BadgerDB** persists the application state.

## Run

```bash
./start.sh
```

The script builds the application, initializes CometBFT when needed, and starts
both processes. Press `Control+C` to stop them.

Runtime data is stored locally and excluded from Git:

```text
data/
├── cometbft/          # Blocks, consensus data, keys, and configuration
├── kvstore/badger/    # Committed KV state
└── mini-comet-chain.sock
```

## Transaction Flow

Submit a `key=value` transaction:

```bash
curl -s 'localhost:26657/broadcast_tx_commit?tx="cometbft=rocks"'
```

The transaction flow is:

```text
RPC request
    ↓
CheckTx validation
    ↓
CometBFT mempool and block consensus
    ↓
FinalizeBlock executes the KV update
    ↓
Commit persists it in BadgerDB
```

Query a value:

```bash
curl -s 'localhost:26657/abci_query?data="cometbft"'
```

ABCI returns keys and values as bytes. CometBFT represents those bytes as
Base64 in JSON. For example, `Y29tZXRiZnQ=` and `cm9ja3M=` decode to
`cometbft` and `rocks`.

## Height and AppHash

For multi-node execution, every validator must reach the same state. The
application will persist two internal values with each block:

```text
height   = latest committed block height
AppHash  = deterministic application state hash
```

The initial hash-chain design is:

```text
new AppHash = SHA-256(
    previous AppHash || block height || successful transactions
)
```

`FinalizeBlock` calculates the new hash. `Commit` atomically stores the KV
updates, height, and hash. After a restart, `Info` returns the saved height and
hash to CometBFT so it can replay any missing blocks.

The calculation must use deterministic byte encoding and block transaction
order. It must not depend on local time, paths, or node-specific data.

## Four-Node Design

The target network has four validators with equal voting power:

```text
Node 1 ───── Node 2
  │            │
Node 3 ───── Node 4

Each node: CometBFT → ABCI → KV application → its own BadgerDB
```

All nodes share the same genesis file, validator set, chain ID, and application
logic. Each node has its own keys, ports, CometBFT home, socket, and BadgerDB.

With four equal validators, at least three votes are required to commit a
block. The network can tolerate one faulty validator and continue with three
online validators.

Every node executes the same ordered transactions independently. Correct,
deterministic execution produces the same height, `AppHash`, and logical KV
state on all four nodes.
