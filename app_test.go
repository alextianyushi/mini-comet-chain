package main

import (
	"bytes"
	"context"
	"testing"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/dgraph-io/badger/v3"
)

func TestHeightAndAppHashPersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, dir)
	app := newTestApp(t, db)

	initial, err := app.Info(context.Background(), &abcitypes.RequestInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if initial.LastBlockHeight != 0 || len(initial.LastBlockAppHash) != 0 {
		t.Fatalf("unexpected initial state: height=%d hash=%x", initial.LastBlockHeight, initial.LastBlockAppHash)
	}

	finalized, err := app.FinalizeBlock(context.Background(), &abcitypes.RequestFinalizeBlock{
		Height: 1,
		Txs:    [][]byte{[]byte("cometbft=rocks")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(finalized.AppHash) != 32 {
		t.Fatalf("AppHash length = %d, want 32", len(finalized.AppHash))
	}
	if _, err := app.Commit(context.Background(), &abcitypes.RequestCommit{}); err != nil {
		t.Fatal(err)
	}

	query, err := app.Query(context.Background(), &abcitypes.RequestQuery{Data: []byte("cometbft")})
	if err != nil {
		t.Fatal(err)
	}
	if query.Height != 1 || !bytes.Equal(query.Value, []byte("rocks")) {
		t.Fatalf("unexpected query: height=%d value=%q", query.Height, query.Value)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTestDB(t, dir)
	defer db.Close()
	restarted := newTestApp(t, db)
	info, err := restarted.Info(context.Background(), &abcitypes.RequestInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if info.LastBlockHeight != 1 || !bytes.Equal(info.LastBlockAppHash, finalized.AppHash) {
		t.Fatalf("state not restored: height=%d hash=%x", info.LastBlockHeight, info.LastBlockAppHash)
	}
}

func TestAppHashIsDeterministic(t *testing.T) {
	txs := [][]byte{[]byte("a=1"), []byte("invalid"), []byte("b=2")}

	hash1 := executeTestBlock(t, t.TempDir(), 7, txs)
	hash2 := executeTestBlock(t, t.TempDir(), 7, txs)
	if !bytes.Equal(hash1, hash2) {
		t.Fatalf("same block produced different hashes: %x != %x", hash1, hash2)
	}

	differentOrder := [][]byte{[]byte("b=2"), []byte("invalid"), []byte("a=1")}
	hash3 := executeTestBlock(t, t.TempDir(), 7, differentOrder)
	if bytes.Equal(hash1, hash3) {
		t.Fatal("different transaction order produced the same hash")
	}
}

func TestCommitRequiresFinalizedBlock(t *testing.T) {
	db := openTestDB(t, t.TempDir())
	defer db.Close()
	app := newTestApp(t, db)

	if _, err := app.Commit(context.Background(), &abcitypes.RequestCommit{}); err == nil {
		t.Fatal("Commit succeeded without FinalizeBlock")
	}
}

func TestProcessProposalRejectsInvalidTransaction(t *testing.T) {
	db := openTestDB(t, t.TempDir())
	defer db.Close()
	app := newTestApp(t, db)

	response, err := app.ProcessProposal(context.Background(), &abcitypes.RequestProcessProposal{
		Txs: [][]byte{[]byte("valid=value"), []byte("invalid")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != abcitypes.ResponseProcessProposal_REJECT {
		t.Fatalf("status = %v, want REJECT", response.Status)
	}
}

func TestSnapshotChunkIsNotAccepted(t *testing.T) {
	db := openTestDB(t, t.TempDir())
	defer db.Close()
	app := newTestApp(t, db)

	response, err := app.ApplySnapshotChunk(context.Background(), &abcitypes.RequestApplySnapshotChunk{})
	if err != nil {
		t.Fatal(err)
	}
	if response.Result != abcitypes.ResponseApplySnapshotChunk_ABORT {
		t.Fatalf("result = %v, want ABORT", response.Result)
	}
}

func executeTestBlock(t *testing.T, dir string, height int64, txs [][]byte) []byte {
	t.Helper()
	db := openTestDB(t, dir)
	defer db.Close()
	app := newTestApp(t, db)
	response, err := app.FinalizeBlock(context.Background(), &abcitypes.RequestFinalizeBlock{
		Height: height,
		Txs:    txs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Commit(context.Background(), &abcitypes.RequestCommit{}); err != nil {
		t.Fatal(err)
	}
	return response.AppHash
}

func openTestDB(t *testing.T, dir string) *badger.DB {
	t.Helper()
	db, err := badger.Open(badger.DefaultOptions(dir).WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func newTestApp(t *testing.T, db *badger.DB) *KVStoreApplication {
	t.Helper()
	app, err := NewKVStoreApplication(db)
	if err != nil {
		t.Fatal(err)
	}
	return app
}
