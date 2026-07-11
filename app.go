package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/dgraph-io/badger/v3"
)

type KVStoreApplication struct {
	db             *badger.DB
	onGoingBlock   *badger.Txn
	mu             sync.RWMutex
	height         int64
	appHash        []byte
	pendingHeight  int64
	pendingAppHash []byte
}

var (
	metadataPrefix = []byte("\x00mini-comet-chain/")
	heightKey      = []byte("\x00mini-comet-chain/height")
	appHashKey     = []byte("\x00mini-comet-chain/app-hash")
)

var _ abcitypes.Application = (*KVStoreApplication)(nil)

func NewKVStoreApplication(db *badger.DB) (*KVStoreApplication, error) {
	app := &KVStoreApplication{db: db}
	if err := app.loadState(); err != nil {
		return nil, err
	}
	return app, nil
}

func (app *KVStoreApplication) Info(_ context.Context, info *abcitypes.RequestInfo) (*abcitypes.ResponseInfo, error) {
	app.mu.RLock()
	defer app.mu.RUnlock()

	return &abcitypes.ResponseInfo{
		LastBlockHeight:  app.height,
		LastBlockAppHash: bytes.Clone(app.appHash),
	}, nil
}

func (app *KVStoreApplication) Query(_ context.Context, req *abcitypes.RequestQuery) (*abcitypes.ResponseQuery, error) {
	app.mu.RLock()
	height := app.height
	app.mu.RUnlock()
	resp := abcitypes.ResponseQuery{Key: req.Data, Height: height}

	dbErr := app.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(req.Data)
		if err != nil {
			if err != badger.ErrKeyNotFound {
				return err
			}
			resp.Log = "key does not exist"
			return nil
		}

		return item.Value(func(val []byte) error {
			resp.Log = "exists"
			resp.Value = val
			return nil
		})
	})
	if dbErr != nil {
		return nil, fmt.Errorf("read query from database: %w", dbErr)
	}
	return &resp, nil
}

func (app *KVStoreApplication) CheckTx(_ context.Context, check *abcitypes.RequestCheckTx) (*abcitypes.ResponseCheckTx, error) {
	code := app.isValid(check.Tx)
	return &abcitypes.ResponseCheckTx{Code: code}, nil
}

func (app *KVStoreApplication) InitChain(_ context.Context, chain *abcitypes.RequestInitChain) (*abcitypes.ResponseInitChain, error) {
	return &abcitypes.ResponseInitChain{}, nil
}

func (app *KVStoreApplication) PrepareProposal(_ context.Context, proposal *abcitypes.RequestPrepareProposal) (*abcitypes.ResponsePrepareProposal, error) {
	return &abcitypes.ResponsePrepareProposal{Txs: proposal.Txs}, nil
}

func (app *KVStoreApplication) ProcessProposal(_ context.Context, proposal *abcitypes.RequestProcessProposal) (*abcitypes.ResponseProcessProposal, error) {
	return &abcitypes.ResponseProcessProposal{Status: abcitypes.ResponseProcessProposal_ACCEPT}, nil
}

func (app *KVStoreApplication) FinalizeBlock(_ context.Context, req *abcitypes.RequestFinalizeBlock) (*abcitypes.ResponseFinalizeBlock, error) {
	app.mu.Lock()
	defer app.mu.Unlock()

	var txs = make([]*abcitypes.ExecTxResult, len(req.Txs))
	var successfulTxs [][]byte

	if app.onGoingBlock != nil {
		app.onGoingBlock.Discard()
	}
	app.onGoingBlock = app.db.NewTransaction(true)
	for i, tx := range req.Txs {
		if code := app.isValid(tx); code != 0 {
			txs[i] = &abcitypes.ExecTxResult{Code: code}
		} else {
			parts := bytes.SplitN(tx, []byte("="), 2)
			key, value := parts[0], parts[1]

			if err := app.onGoingBlock.Set(key, value); err != nil {
				app.onGoingBlock.Discard()
				app.onGoingBlock = nil
				return nil, fmt.Errorf("prepare KV update: %w", err)
			}
			successfulTxs = append(successfulTxs, tx)
			txs[i] = &abcitypes.ExecTxResult{}
		}
	}

	newAppHash := calculateAppHash(app.appHash, req.Height, successfulTxs)
	if err := app.onGoingBlock.Set(heightKey, encodeHeight(req.Height)); err != nil {
		app.onGoingBlock.Discard()
		app.onGoingBlock = nil
		return nil, fmt.Errorf("prepare application height: %w", err)
	}
	if err := app.onGoingBlock.Set(appHashKey, newAppHash); err != nil {
		app.onGoingBlock.Discard()
		app.onGoingBlock = nil
		return nil, fmt.Errorf("prepare application hash: %w", err)
	}
	app.pendingHeight = req.Height
	app.pendingAppHash = newAppHash

	return &abcitypes.ResponseFinalizeBlock{
		TxResults: txs,
		AppHash:   bytes.Clone(newAppHash),
	}, nil
}

func (app *KVStoreApplication) Commit(_ context.Context, commit *abcitypes.RequestCommit) (*abcitypes.ResponseCommit, error) {
	app.mu.Lock()
	defer app.mu.Unlock()

	if app.onGoingBlock == nil {
		return nil, fmt.Errorf("commit called without a finalized block")
	}
	if err := app.onGoingBlock.Commit(); err != nil {
		app.onGoingBlock = nil
		return nil, fmt.Errorf("commit application state: %w", err)
	}

	app.onGoingBlock = nil
	app.height = app.pendingHeight
	app.appHash = bytes.Clone(app.pendingAppHash)
	return &abcitypes.ResponseCommit{}, nil
}

func (app *KVStoreApplication) ListSnapshots(_ context.Context, snapshots *abcitypes.RequestListSnapshots) (*abcitypes.ResponseListSnapshots, error) {
	return &abcitypes.ResponseListSnapshots{}, nil
}

func (app *KVStoreApplication) OfferSnapshot(_ context.Context, snapshot *abcitypes.RequestOfferSnapshot) (*abcitypes.ResponseOfferSnapshot, error) {
	return &abcitypes.ResponseOfferSnapshot{}, nil
}

func (app *KVStoreApplication) LoadSnapshotChunk(_ context.Context, chunk *abcitypes.RequestLoadSnapshotChunk) (*abcitypes.ResponseLoadSnapshotChunk, error) {
	return &abcitypes.ResponseLoadSnapshotChunk{}, nil
}

func (app *KVStoreApplication) ApplySnapshotChunk(_ context.Context, chunk *abcitypes.RequestApplySnapshotChunk) (*abcitypes.ResponseApplySnapshotChunk, error) {

	return &abcitypes.ResponseApplySnapshotChunk{Result: abcitypes.ResponseApplySnapshotChunk_ACCEPT}, nil
}

func (app *KVStoreApplication) ExtendVote(_ context.Context, extend *abcitypes.RequestExtendVote) (*abcitypes.ResponseExtendVote, error) {
	return &abcitypes.ResponseExtendVote{}, nil
}

func (app *KVStoreApplication) VerifyVoteExtension(_ context.Context, verify *abcitypes.RequestVerifyVoteExtension) (*abcitypes.ResponseVerifyVoteExtension, error) {
	return &abcitypes.ResponseVerifyVoteExtension{}, nil
}

func (app *KVStoreApplication) isValid(tx []byte) uint32 {
	// check format
	parts := bytes.Split(tx, []byte("="))
	if len(parts) != 2 || bytes.HasPrefix(parts[0], metadataPrefix) {
		return 1
	}
	return 0
}

func (app *KVStoreApplication) loadState() error {
	return app.db.View(func(txn *badger.Txn) error {
		height, heightFound, err := readValue(txn, heightKey)
		if err != nil {
			return fmt.Errorf("read application height: %w", err)
		}
		appHash, hashFound, err := readValue(txn, appHashKey)
		if err != nil {
			return fmt.Errorf("read application hash: %w", err)
		}
		if heightFound != hashFound {
			return fmt.Errorf("incomplete application metadata")
		}
		if !heightFound {
			return nil
		}
		if len(height) != 8 {
			return fmt.Errorf("invalid encoded application height")
		}
		if len(appHash) != sha256.Size {
			return fmt.Errorf("invalid application hash length %d", len(appHash))
		}
		app.height = int64(binary.BigEndian.Uint64(height))
		app.appHash = appHash
		return nil
	})
}

func readValue(txn *badger.Txn, key []byte) ([]byte, bool, error) {
	item, err := txn.Get(key)
	if err == badger.ErrKeyNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	value, err := item.ValueCopy(nil)
	return value, true, err
}

func encodeHeight(height int64) []byte {
	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, uint64(height))
	return encoded
}

func calculateAppHash(previousHash []byte, height int64, txs [][]byte) []byte {
	hash := sha256.New()
	hash.Write([]byte("mini-comet-chain/app-hash/v1"))
	writeHashField(hash, previousHash)
	hash.Write(encodeHeight(height))

	count := make([]byte, 8)
	binary.BigEndian.PutUint64(count, uint64(len(txs)))
	hash.Write(count)
	for _, tx := range txs {
		writeHashField(hash, tx)
	}
	return hash.Sum(nil)
}

type hashWriter interface {
	Write([]byte) (int, error)
}

func writeHashField(hash hashWriter, value []byte) {
	length := make([]byte, 8)
	binary.BigEndian.PutUint64(length, uint64(len(value)))
	hash.Write(length)
	hash.Write(value)
}
