# Mini Comet Chain

A KV store blockchain built with Go, CometBFT v0.39.0, ABCI, and BadgerDB.

## Components

- **CometBFT** orders transactions, produces blocks, and runs consensus.
- **KV application** validates and executes `key=value` transactions.
- **ABCI** connects CometBFT to the KV application.
- **BadgerDB** persists the application state.

## Run

Create the four validator configurations once:

```bash
./setup-four-nodes.sh
```

Then start all four KV applications and CometBFT nodes:

```bash
./start-four-nodes.sh
```

Press `Control+C` to stop the complete network. To create a new chain, remove
`data/` and run the setup script again.

RPC endpoints:

```text
node1  http://127.0.0.1:26657
node2  http://127.0.0.1:26757
node3  http://127.0.0.1:26857
node4  http://127.0.0.1:26957
```

Runtime data is stored locally and excluded from Git:

```text
data/
├── node1/{cometbft,kvstore}
├── node2/{cometbft,kvstore}
├── node3/{cometbft,kvstore}
└── node4/{cometbft,kvstore}
```

## Transaction Flow

Submit a `key=value` transaction to any node. This example uses node1:

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

After the transaction is committed and synchronized, the value can be queried
from any node:

```bash
curl -s 'localhost:26657/abci_query?data="cometbft"'
curl -s 'localhost:26757/abci_query?data="cometbft"'
curl -s 'localhost:26857/abci_query?data="cometbft"'
curl -s 'localhost:26957/abci_query?data="cometbft"'
```

All four queries return the same key and value because every validator executes
the same committed transactions and stores the same logical KV state. The
reported height is the latest application height at the moment each query is
handled.

ABCI returns keys and values as bytes. CometBFT represents those bytes as
Base64 in JSON. For example, `Y29tZXRiZnQ=` and `cm9ja3M=` decode to
`cometbft` and `rocks`.

## Height and AppHash

For multi-node execution, every validator must reach the same state. The
application persists two internal values with each block:

```text
height   = latest committed block height
AppHash  = deterministic application execution hash
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

An AppHash is recorded in the next block header. For example:

```text
Execute block 10
    ↓
Calculate AppHash 10
    ↓
Store AppHash 10 in the header of block 11
```

This one-block delay exists because the header of block 10 is created before
the application executes block 10. Therefore, the execution result of block
`H` is committed as `app_hash` in block `H+1`.

The calculation must use deterministic byte encoding and block transaction
order. It must not depend on local time, paths, or node-specific data.

This hash commits to the execution history. It is not a Merkle root of every
KV currently stored in BadgerDB.

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

## Tests

Run the KV application tests:

```bash
go test ./...
```

The tests cover height and AppHash persistence, restart recovery, deterministic
hashing, proposal validation, commit order, and unsupported snapshot handling.
They use temporary BadgerDB directories and do not modify `data/`.

Run the race detector with:

```bash
go test -race ./...
```

The four-node network checks are currently performed through
`setup-four-nodes.sh` and `start-four-nodes.sh`.

## Reference

See `docs/cometbft-kvstore-reference.md` for the adapted CometBFT KV store
tutorial. It is background material; the commands in this README are the
current instructions for this repository.
