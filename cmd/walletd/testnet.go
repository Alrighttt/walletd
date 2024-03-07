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

// TestnetAnagami returns the chain parameters and genesis block for the "Anagami"
// testnet chain.
func TestnetAnagami() (*consensus.Network, types.Block) {
	n := &consensus.Network{
		Name: "anagami",

		InitialCoinbase: types.Siacoins(300000),
		MinimumCoinbase: types.Siacoins(300000),
		InitialTarget:   types.BlockID{3: 1},
	}

	n.HardforkDevAddr.Height = 1
	n.HardforkDevAddr.OldAddress = types.Address{}
	n.HardforkDevAddr.NewAddress = types.Address{}

	n.HardforkTax.Height = 2

	n.HardforkStorageProof.Height = 3

	n.HardforkOak.Height = 5
	n.HardforkOak.FixHeight = 8
	n.HardforkOak.GenesisTimestamp = time.Unix(1702300000, 0) // Dec 11, 2023 @ 13:06 GMT

	n.HardforkASIC.Height = 13
	n.HardforkASIC.OakTime = 10 * time.Minute
	n.HardforkASIC.OakTarget = n.InitialTarget

	n.HardforkFoundation.Height = 21
	n.HardforkFoundation.PrimaryAddress, _ = types.ParseAddress("addr:5949fdf56a7c18ba27f6526f22fd560526ce02a1bd4fa3104938ab744b69cf63b6b734b8341f")
	n.HardforkFoundation.FailsafeAddress = n.HardforkFoundation.PrimaryAddress

	n.HardforkV2.AllowHeight = 2016         // ~2 weeks in
	n.HardforkV2.RequireHeight = 2016 + 288 // ~2 days later

	b := types.Block{
		Timestamp: n.HardforkOak.GenesisTimestamp,
		Transactions: []types.Transaction{{
			SiacoinOutputs: []types.SiacoinOutput{{
				Address: n.HardforkFoundation.PrimaryAddress,
				Value:   types.Siacoins(1).Mul64(1e12),
			}},
			SiafundOutputs: []types.SiafundOutput{{
				Address: n.HardforkFoundation.PrimaryAddress,
				Value:   10000,
			}},
		}},
	}

	return n, b
}

// TestnetAnagami returns the chain parameters and genesis block for the "Anagami"
// testnet chain.
func TestnetKomodo() (*consensus.Network, types.Block) {
	n := &consensus.Network{
		Name: "komodo",

		InitialCoinbase: types.Siacoins(300000),
		MinimumCoinbase: types.Siacoins(300000),
		InitialTarget:   types.BlockID{0: 1}, // significantly reduced POW diff
	}

	n.HardforkDevAddr.Height = 1
	n.HardforkDevAddr.OldAddress = types.Address{}
	n.HardforkDevAddr.NewAddress = types.Address{}

	n.HardforkTax.Height = 2

	n.HardforkStorageProof.Height = 3

	n.HardforkOak.Height = 5
	n.HardforkOak.FixHeight = 8
	n.HardforkOak.GenesisTimestamp = time.Unix(1702300000, 0) // Dec 11, 2023 @ 13:06 GMT

	n.HardforkASIC.Height = 13
	n.HardforkASIC.OakTime = 10 * time.Minute
	n.HardforkASIC.OakTarget = n.InitialTarget

	n.HardforkFoundation.Height = 21
	n.HardforkFoundation.PrimaryAddress, _ = types.ParseAddress("addr:591fcf237f8854b5653d1ac84ae4c107b37f148c3c7b413f292d48db0c25a8840be0653e411f") // Address controlled by Alrighttt
	n.HardforkFoundation.FailsafeAddress = n.HardforkFoundation.PrimaryAddress

	n.HardforkV2.AllowHeight = 10        // activate immediately
	n.HardforkV2.RequireHeight = 7777777 // remain in flux state v1/v2 transition indefinitely

	b := types.Block{
		Timestamp: n.HardforkOak.GenesisTimestamp,
		Transactions: []types.Transaction{{
			SiacoinOutputs: []types.SiacoinOutput{{
				Address: n.HardforkFoundation.PrimaryAddress,
				Value:   types.Siacoins(1).Mul64(1e12),
			}},
			SiafundOutputs: []types.SiafundOutput{{
				Address: n.HardforkFoundation.PrimaryAddress,
				Value:   10000,
			}},
		}},
	}

	return n, b
}

func loadTestnetSeed(s string) wallet.Seed {
	if s == "" {
		fmt.Println("Seed not supplied via -seed flag, falling back to manual entry.")
		fmt.Print("Seed: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		check("Could not read API password:", err)
		if err != nil {
			log.Fatal(err)
		}
		s = string(pw)
	}
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 8 {
		log.Fatal("Seed must be 16 hex characters")
	}
	var entropy [32]byte
	copy(entropy[:], b)
	return wallet.NewSeedFromEntropy(&entropy)
}

func initTestnetClient(addr string, network string, seed wallet.Seed) *api.Client {
	if network == "mainnet" {
		log.Fatal("Testnet actions cannot be used on mainnet")
	}
	c := api.NewClient("http://"+addr+"/api", getAPIPassword())
	cs, err := c.ConsensusTipState()
	check("Couldn't connect to API:", err)
	if cs.Network.Name != network {
		log.Fatalf("Testnet %q was specified, but walletd is running %v", network, cs.Network.Name)
	}
	ourAddr := types.StandardUnlockHash(seed.PublicKey(0))
	wc := c.Wallet("primary")
	if addrs, err := wc.Addresses(); err == nil && len(addrs) > 0 {
		if _, ok := addrs[ourAddr]; !ok {
			log.Fatal("Wallet already initialized with a different testnet address")
		}
	}
	if ws, _ := c.Wallets(); len(ws) == 0 {
		fmt.Print("Initializing testnet wallet...")
		if err := c.AddWallet("primary", nil); err != nil {
			fmt.Println()
			log.Fatal(err)
		} else if err := wc.AddAddress(ourAddr, nil); err != nil {
			fmt.Println()
			log.Fatal(err)
		} else if err := wc.Subscribe(0); err != nil {
			fmt.Println()
			log.Fatal(err)
		}
		fmt.Println("done.")
	}
	return c
}

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

func runTestnetMiner(c *api.Client, seed wallet.Seed, blocksToMine int) {
	minerAddr := types.StandardUnlockHash(seed.PublicKey(0))

	log.Println("Started mining into", minerAddr)
	start := time.Now()

	var hashes float64
	var blocks uint64
	var last types.ChainIndex
	minedCount := 0
outer:
	for i := 0; ; i++ {
		if n <= 0 && i >= n {
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
			minedCount += 1
			fmt.Printf("\nFound v1 block %v\n", index)
		} else {
			minedCount += 1
			fmt.Printf("\nFound v2 block %v\n", index)
		}
		if blocksToMine != 0 && minedCount > blocksToMine {
			break outer
		}
	}
}
