// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	Tx "github.com/vechain/thor/tx"
)

func init() {
	log15.Root().SetHandler(log15.DiscardHandler())
}

func newPool() *TxPool {
	db := muxdb.NewMem()
	repo := newChainRepo(db)
	return New(repo, state.NewStater(db), Options{
		Limit:           10,
		LimitPerAccount: 2,
		MaxLifetime:     time.Hour,
	})
}
func TestNewClose(t *testing.T) {
	pool := newPool()
	defer pool.Close()
}

func TestSubscribeNewTx(t *testing.T) {
	pool := newPool()
	defer pool.Close()

	b1 := new(block.Builder).
		ParentID(pool.repo.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(pool.repo.GenesisBlock().Header().StateRoot()).
		Build()
	pool.repo.AddBlock(b1, nil)
	pool.repo.SetBestBlockID(b1.Header().ID())

	txCh := make(chan *TxEvent)

	pool.SubscribeTxEvent(txCh)

	tx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	assert.Nil(t, pool.Add(tx, false))

	v := true
	assert.Equal(t, &TxEvent{tx, &v}, <-txCh)
}

func TestWashTxs(t *testing.T) {
	pool := newPool()
	defer pool.Close()
	txs, _, err := pool.wash(pool.repo.BestBlock().Header())
	assert.Nil(t, err)
	assert.Zero(t, len(txs))
	assert.Zero(t, len(pool.Executables()))

	tx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), genesis.DevAccounts()[0])
	assert.Nil(t, pool.Add(tx, false))

	txs, _, err = pool.wash(pool.repo.BestBlock().Header())
	assert.Nil(t, err)
	assert.Equal(t, Tx.Transactions{tx}, txs)

	b1 := new(block.Builder).
		ParentID(pool.repo.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(pool.repo.GenesisBlock().Header().StateRoot()).
		Build()
	pool.repo.AddBlock(b1, nil)

	txs, _, err = pool.wash(pool.repo.BestBlock().Header())
	assert.Nil(t, err)
	assert.Equal(t, Tx.Transactions{tx}, txs)
}

func TestAdd(t *testing.T) {
	pool := newPool()
	defer pool.Close()
	b1 := new(block.Builder).
		ParentID(pool.repo.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(pool.repo.GenesisBlock().Header().StateRoot()).
		Build()
	pool.repo.AddBlock(b1, nil)
	pool.repo.SetBestBlockID(b1.Header().ID())
	acc := genesis.DevAccounts()[0]

	dupTx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)

	tests := []struct {
		tx     *tx.Transaction
		errStr string
	}{
		{newTx(pool.repo.ChainTag()+1, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc), "bad tx: chain tag mismatch"},
		{dupTx, ""},
		{dupTx, ""},
	}

	for _, tt := range tests {
		err := pool.Add(tt.tx, false)
		if tt.errStr == "" {
			assert.Nil(t, err)
		} else {
			assert.Equal(t, tt.errStr, err.Error())
		}
	}

	raw, _ := hex.DecodeString("f8dc81a484aabbccdd20f840df947567d83b7b8d80addcb281a71d54fc7b3364ffed82271086000000606060df947567d83b7b8d80addcb281a71d54fc7b3364ffed824e20860000006060608180830334508083bc614ec20108b88256e32450c1907f627d2c11fe5a9d0216be1712f4938b5feb04e37edef236c56266c3378acf97994beff22698b70023f486645d29cb23b479a7b044f7c6b104d2000584fcb3964446d4d832dcc849e2d76ea7e04a4ebdc3a4b61e7997e93277363d4e7fe9315e7f6dd8d9c0a8bff5879503f5c04adab8b08772499e74d34f67923501")
	var badReserved *Tx.Transaction
	if err := rlp.DecodeBytes(raw, &badReserved); err != nil {
		t.Error(err)
	}

	tests = []struct {
		tx     *Tx.Transaction
		errStr string
	}{
		{newTx(pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(200), 100, nil, Tx.Features(0), acc), "tx rejected: tx is not executable"},
		{newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(0), acc), "tx rejected: tx is not executable"},
		{newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(2), acc), "tx rejected: unsupported features"},
		{badReserved, "tx rejected: unsupported features"},
	}

	for _, tt := range tests {
		err := pool.StrictlyAdd(tt.tx, false)
		if tt.errStr == "" {
			assert.Nil(t, err)
		} else {
			assert.Equal(t, tt.errStr, err.Error())
		}
	}
}

func TestBeforeVIP191Add(t *testing.T) {
	db := muxdb.NewMem()
	defer db.Close()

	chain := newChainRepo(db)

	acc := genesis.DevAccounts()[0]

	pool := New(chain, state.NewStater(db), Options{
		Limit:           10,
		LimitPerAccount: 2,
		MaxLifetime:     time.Hour,
	})
	defer pool.Close()

	err := pool.StrictlyAdd(newTx(pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(200), 100, nil, Tx.Features(1), acc), false)

	assert.Equal(t, "tx rejected: unsupported features", err.Error())
}
