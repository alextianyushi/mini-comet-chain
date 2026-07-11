# Tests

The tests are located in `app_test.go`.

Run all tests:

```bash
go test ./...
```

A successful result looks like:

```text
ok      mini-comet-chain      0.535s
```

## Current Tests

### Height, AppHash, and Restart

```text
TestHeightAndAppHashPersistAcrossRestart
```

This test:

1. Executes `cometbft=rocks` at height 1.
2. Commits the KV value, height, and AppHash.
3. Checks that the query returns `rocks` at height 1.
4. Reopens BadgerDB and checks that the same height and AppHash are restored.

### Deterministic AppHash

```text
TestAppHashIsDeterministic
```

This test confirms that:

- The same height and transactions produce the same AppHash.
- A different transaction order produces a different AppHash.
- Invalid transactions are not included as successful transactions.

### Commit Order

```text
TestCommitRequiresFinalizedBlock
```

This test confirms that `Commit` fails when `FinalizeBlock` has not been called.

The required order is:

```text
FinalizeBlock → Commit
```

## Test Data

Each test uses a temporary BadgerDB created with `t.TempDir()`. The tests do not
modify the four-node data under `data/`.

## Useful Commands

Show each test:

```bash
go test -v ./...
```

Run one test:

```bash
go test -run TestAppHashIsDeterministic
```

Check for data races:

```bash
go test -race ./...
```
