package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math/big"
	"time"

	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
	"go.sia.tech/walletd/api"
	"lukechampine.com/frand"
)

func mineBlock(cs consensus.State, b *types.Block) (hashes int, found bool) {
	buf := make([]byte, 32+8+8+32)
	binary.LittleEndian.PutUint64(buf[32:], b.Nonce)
	binary.LittleEndian.PutUint64(buf[40:], uint64(b.Timestamp.Unix()))
	if b.V2 != nil {
		copy(buf[:32], "sia/id/block|")
		copy(buf[48:], b.V2.Commitment[:])
	} else {
		root := b.MerkleRoot()
		copy(buf[:32], b.ParentID[:])
		copy(buf[48:], root[:])
	}
	factor := cs.NonceFactor()
	startBlock := time.Now()
	for types.BlockID(types.HashBytes(buf)).CmpWork(cs.ChildTarget) < 0 {
		b.Nonce += factor
		hashes++
		binary.LittleEndian.PutUint64(buf[32:], b.Nonce)
		if time.Since(startBlock) > 10*time.Second {
			return hashes, false
		}
	}
	return hashes, true
}

func runCPUMiner(c *api.Client, minerAddr types.Address, n int) {
	log.Println("Started mining into", minerAddr)
	start := time.Now()

	var hashes float64
	var blocks uint64
	var last types.ChainIndex
outer:
	for i := 0; ; i++ {
		if n >= 0 && i >= n {
			return
		}
		elapsed := time.Since(start)
		cs, err := c.ConsensusTipState()
		check("Couldn't get consensus tip state:", err)
		if cs.Index == last {
			fmt.Println("Tip now", cs.Index)
			last = cs.Index
		}
		n := big.NewInt(int64(hashes))
		n.Mul(n, big.NewInt(int64(24*time.Hour)))
		d, _ := new(big.Int).SetString(cs.Difficulty.String(), 10)
		d.Mul(d, big.NewInt(int64(1+elapsed)))
		r, _ := new(big.Rat).SetFrac(n, d).Float64()
		fmt.Printf("\rMining block %4v...(%.2f kH/s, %.2f blocks/day (expected: %.2f), difficulty %v)", cs.Index.Height+1, hashes/elapsed.Seconds()/1000, float64(blocks)*float64(24*time.Hour)/float64(elapsed), r, cs.Difficulty)

		txns, v2txns, err := c.TxpoolTransactions()
		check("Couldn't get txpool transactions:", err)
		b := types.Block{
			ParentID:     cs.Index.ID,
			Nonce:        cs.NonceFactor() * frand.Uint64n(100),
			Timestamp:    types.CurrentTimestamp(),
			MinerPayouts: []types.SiacoinOutput{{Address: minerAddr, Value: cs.BlockReward()}},
			Transactions: txns,
		}
		for _, txn := range txns {
			b.MinerPayouts[0].Value = b.MinerPayouts[0].Value.Add(txn.TotalFees())
		}
		for _, txn := range v2txns {
			b.MinerPayouts[0].Value = b.MinerPayouts[0].Value.Add(txn.MinerFee)
		}
		if len(v2txns) > 0 || cs.Index.Height+1 >= cs.Network.HardforkV2.RequireHeight {
			b.V2 = &types.V2BlockData{
				Height:       cs.Index.Height + 1,
				Transactions: v2txns,
			}
			b.V2.Commitment = cs.Commitment(cs.TransactionsCommitment(b.Transactions, b.V2Transactions()), b.MinerPayouts[0].Address)
		}
		h, ok := mineBlock(cs, &b)
		hashes += float64(h)
		if !ok {
			continue outer
		}
		blocks++
		index := types.ChainIndex{Height: cs.Index.Height + 1, ID: b.ID()}
		tip, err := c.ConsensusTip()
		check("Couldn't get consensus tip:", err)
		if tip != cs.Index {
			fmt.Printf("\nMined %v but tip changed, starting over\n", index)
		} else if err := c.SyncerBroadcastBlock(b); err != nil {
			fmt.Printf("\nMined invalid block: %v\n", err)
		} else if b.V2 == nil {
			fmt.Printf("\nFound v1 block %v\n", index)
		} else {
			fmt.Printf("\nFound v2 block %v\n", index)
		}
	}
}
