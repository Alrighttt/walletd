package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"
	"go.sia.tech/core/chain"
	"go.sia.tech/core/consensus"
	"go.sia.tech/core/gateway"
	"go.sia.tech/core/types"
	"go.sia.tech/walletd/internal/syncerutil"
	"go.sia.tech/walletd/internal/walletutil"
	"go.sia.tech/walletd/syncer"
	"lukechampine.com/upnp"
)

var mainnetBootstrap = []string{
	"1.1.1.1:9981",
}

var zenBootstrap = []string{
	"147.135.16.182:9881",
	"147.135.39.109:9881",
	"51.81.208.10:9881",
}

var anagamiBootstrap = []string{
	"1.1.1.1:9981",
}

type boltDB struct {
	tx *bolt.Tx
	db *bolt.DB
}

func (db *boltDB) newTx() (err error) {
	if db.tx == nil {
		db.tx, err = db.db.Begin(true)
	}
	return
}

func (db *boltDB) Bucket(name []byte) chain.DBBucket {
	if err := db.newTx(); err != nil {
		panic(err)
	}

	b := db.tx.Bucket(name)
	if b == nil {
		return nil
	}
	return b
}

func (db *boltDB) CreateBucket(name []byte) (chain.DBBucket, error) {
	if err := db.newTx(); err != nil {
		return nil, err
	}

	b, err := db.tx.CreateBucket(name)
	if b == nil {
		return nil, err
	}
	return b, nil
}

func (db *boltDB) Flush() error {
	if db.tx == nil {
		return nil
	}

	if err := db.tx.Commit(); err != nil {
		return err
	}
	db.tx = nil
	return nil
}

func (db *boltDB) Cancel() {
	if db.tx == nil {
		return
	}

	db.tx.Rollback()
	db.tx = nil
}

func (db *boltDB) Close() error {
	db.Flush()
	return db.db.Close()
}

type node struct {
	cm *chain.Manager
	s  *syncer.Syncer
	wm *walletutil.JSONWalletManager

	Start func() (stop func())
}

func newNode(addr, dir string, chainNetwork string, useUPNP bool) (*node, error) {
	var network *consensus.Network
	var genesisBlock types.Block
	var bootstrapPeers []string
	switch chainNetwork {
	case "mainnet":
		network, genesisBlock = chain.Mainnet()
		bootstrapPeers = mainnetBootstrap
	case "zen":
		network, genesisBlock = chain.TestnetZen()
		bootstrapPeers = zenBootstrap
	case "anagami":
		network, genesisBlock = TestnetAnagami()
		bootstrapPeers = anagamiBootstrap
		testnetFixDBTree(dir)
		testnetFixMultiproofs(dir)
	default:
		return nil, errors.New("invalid network: must be one of 'mainnet', 'zen', or 'anagami'")
	}

	bdb, err := bolt.Open(filepath.Join(dir, "consensus.db"), 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	db := &boltDB{db: bdb}
	dbstore, tipState, err := chain.NewDBStore(db, network, genesisBlock)
	if err != nil {
		return nil, err
	}
	cm := chain.NewManager(dbstore, tipState)

	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	syncerAddr := l.Addr().String()
	if useUPNP {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if d, err := upnp.Discover(ctx); err != nil {
			log.Println("WARN: couldn't discover UPnP device:", err)
		} else {
			_, portStr, _ := net.SplitHostPort(addr)
			port, _ := strconv.Atoi(portStr)
			if !d.IsForwarded(uint16(port), "TCP") {
				if err := d.Forward(uint16(port), "TCP", "walletd"); err != nil {
					log.Println("WARN: couldn't forward port:", err)
				} else {
					log.Println("p2p: Forwarded port", port)
				}
			}
			if ip, err := d.ExternalIP(); err != nil {
				log.Println("WARN: couldn't determine external IP:", err)
			} else {
				log.Println("p2p: External IP is", ip)
				syncerAddr = net.JoinHostPort(ip, portStr)
			}
		}
	}
	// peers will reject us if our hostname is empty or unspecified, so use loopback
	host, port, _ := net.SplitHostPort(syncerAddr)
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		syncerAddr = net.JoinHostPort("127.0.0.1", port)
	}

	ps, err := syncerutil.NewJSONPeerStore(filepath.Join(dir, "peers.json"))
	if err != nil {
		log.Fatal(err)
	}
	for _, peer := range bootstrapPeers {
		ps.AddPeer(peer)
	}
	header := gateway.Header{
		GenesisID:  genesisBlock.ID(),
		UniqueID:   gateway.GenerateUniqueID(),
		NetAddress: syncerAddr,
	}
	logFile, err := os.OpenFile(filepath.Join(dir, "walletd.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	logger := log.New(io.MultiWriter(os.Stderr, logFile), "", log.LstdFlags)
	s := syncer.New(l, cm, ps, header, syncer.WithLogger(logger))

	wm, err := walletutil.NewJSONWalletManager(dir, cm)
	if err != nil {
		return nil, err
	}

	return &node{
		cm: cm,
		s:  s,
		wm: wm,
		Start: func() func() {
			ch := make(chan struct{})
			go func() {
				s.Run()
				close(ch)
			}()
			return func() {
				l.Close()
				<-ch
				db.Close()
			}
		},
	}, nil
}
