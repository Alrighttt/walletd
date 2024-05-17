package wallet

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
	"go.sia.tech/walletd/internal/threadgroup"
	"go.uber.org/zap"
)

// IndexMode represents the index mode of the wallet manager. The index mode
// determines how the wallet manager stores the consensus state.
//
// IndexModePersonal - The wallet manager scans the blockchain starting at
// genesis. Only state from addresses that are registered with a
// wallet will be stored. If an address is added to a wallet after the
// scan completes, the manager will need to rescan.
//
// IndexModeFull - The wallet manager scans the blockchain starting at genesis
// and stores the state of all addresses.
//
// IndexModeNone - The wallet manager does not scan the blockchain. This is
// useful for multiple nodes sharing the same database. None should only be used
// when connecting to a database that is in "Full" mode.
const (
	IndexModePersonal IndexMode = iota
	IndexModeFull
	IndexModeNone
)

const defaultSyncBatchSize = 1

type (
	// An IndexMode determines the chain state that the wallet manager stores.
	IndexMode uint8

	// A ChainManager manages the consensus state
	ChainManager interface {
		Tip() types.ChainIndex
		BestIndex(height uint64) (types.ChainIndex, bool)

		OnReorg(fn func(types.ChainIndex)) (cancel func())
		UpdatesSince(index types.ChainIndex, max int) (rus []chain.RevertUpdate, aus []chain.ApplyUpdate, err error)
	}

	// A Store is a persistent store of wallet data.
	Store interface {
		UpdateChainState(reverted []chain.RevertUpdate, applied []chain.ApplyUpdate) error

		WalletEvents(walletID ID, offset, limit int) ([]Event, error)
		AddWallet(Wallet) (Wallet, error)
		UpdateWallet(Wallet) (Wallet, error)
		DeleteWallet(walletID ID) error
		WalletBalance(walletID ID) (Balance, error)
		WalletSiacoinOutputs(walletID ID, offset, limit int) ([]types.SiacoinElement, error)
		WalletSiafundOutputs(walletID ID, offset, limit int) ([]types.SiafundElement, error)
		WalletAddresses(walletID ID) ([]Address, error)
		Wallets() ([]Wallet, error)

		AddWalletAddress(walletID ID, address Address) error
		RemoveWalletAddress(walletID ID, address types.Address) error

		Annotate(walletID ID, txns []types.Transaction) ([]PoolTransaction, error)

		AddressBalance(address types.Address) (balance Balance, err error)
		AddressEvents(address types.Address, offset, limit int) (events []Event, err error)
		AddressSiacoinOutputs(address types.Address, offset, limit int) (siacoins []types.SiacoinElement, err error)
		AddressSiafundOutputs(address types.Address, offset, limit int) (siafunds []types.SiafundElement, err error)

		SetIndexMode(IndexMode) error
		LastCommittedIndex() (types.ChainIndex, error)
	}

	// A Manager manages wallets.
	Manager struct {
		indexMode     IndexMode
		syncBatchSize int

		chain ChainManager
		store Store
		log   *zap.Logger
		tg    *threadgroup.ThreadGroup

		mu   sync.Mutex // protects the fields below
		used map[types.Hash256]bool
	}
)

// String returns the string representation of the index mode.
func (i IndexMode) String() string {
	switch i {
	case IndexModePersonal:
		return "personal"
	case IndexModeFull:
		return "full"
	case IndexModeNone:
		return "none"
	default:
		return "unknown"
	}
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (i *IndexMode) UnmarshalText(buf []byte) error {
	switch string(buf) {
	case "personal":
		*i = IndexModePersonal
	case "full":
		*i = IndexModeFull
	case "none":
		*i = IndexModeNone
	default:
		return fmt.Errorf("unknown index mode %q", buf)
	}
	return nil
}

// MarshalText implements the encoding.TextMarshaler interface.
func (i IndexMode) MarshalText() ([]byte, error) {
	return []byte(i.String()), nil
}

// Tip returns the last scanned chain index of the manager.
func (m *Manager) Tip() (types.ChainIndex, error) {
	return m.store.LastCommittedIndex()
}

// AddWallet adds the given wallet.
func (m *Manager) AddWallet(w Wallet) (Wallet, error) {
	return m.store.AddWallet(w)
}

// UpdateWallet updates the given wallet.
func (m *Manager) UpdateWallet(w Wallet) (Wallet, error) {
	return m.store.UpdateWallet(w)
}

// DeleteWallet deletes the given wallet.
func (m *Manager) DeleteWallet(walletID ID) error {
	return m.store.DeleteWallet(walletID)
}

// Wallets returns the wallets of the wallet manager.
func (m *Manager) Wallets() ([]Wallet, error) {
	return m.store.Wallets()
}

// AddAddress adds the given address to the given wallet.
func (m *Manager) AddAddress(walletID ID, addr Address) error {
	return m.store.AddWalletAddress(walletID, addr)
}

// RemoveAddress removes the given address from the given wallet.
func (m *Manager) RemoveAddress(walletID ID, addr types.Address) error {
	return m.store.RemoveWalletAddress(walletID, addr)
}

// Addresses returns the addresses of the given wallet.
func (m *Manager) Addresses(walletID ID) ([]Address, error) {
	return m.store.WalletAddresses(walletID)
}

// Events returns the events of the given wallet.
func (m *Manager) Events(walletID ID, offset, limit int) ([]Event, error) {
	return m.store.WalletEvents(walletID, offset, limit)
}

// UnspentSiacoinOutputs returns a paginated list of unspent siacoin outputs of
// the given wallet and the total number of unspent siacoin outputs.
func (m *Manager) UnspentSiacoinOutputs(walletID ID, offset, limit int) ([]types.SiacoinElement, error) {
	return m.store.WalletSiacoinOutputs(walletID, offset, limit)
}

// UnspentSiafundOutputs returns the unspent siafund outputs of the given wallet
func (m *Manager) UnspentSiafundOutputs(walletID ID, offset, limit int) ([]types.SiafundElement, error) {
	return m.store.WalletSiafundOutputs(walletID, offset, limit)
}

// Annotate annotates the given transactions with the wallet they belong to.
func (m *Manager) Annotate(walletID ID, pool []types.Transaction) ([]PoolTransaction, error) {
	return m.store.Annotate(walletID, pool)
}

// WalletBalance returns the balance of the given wallet.
func (m *Manager) WalletBalance(walletID ID) (Balance, error) {
	return m.store.WalletBalance(walletID)
}

// Reserve reserves the given ids for the given duration.
func (m *Manager) Reserve(ids []types.Hash256, duration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// check if any of the ids are already reserved
	for _, id := range ids {
		if m.used[id] {
			return fmt.Errorf("output %q already reserved", id)
		}
	}

	// reserve the ids
	for _, id := range ids {
		m.used[id] = true
	}

	// sleep for the duration and then unreserve the ids
	time.AfterFunc(duration, func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		for _, id := range ids {
			delete(m.used, id)
		}
	})
	return nil
}

// Scan rescans the chain starting from the given index. The scan will complete
// when the chain manager reaches the current tip or the context is canceled.
func (m *Manager) Scan(ctx context.Context, index types.ChainIndex) error {
	if m.indexMode != IndexModePersonal {
		return fmt.Errorf("scans are disabled in index mode %s", m.indexMode)
	}

	ctx, cancel, err := m.tg.AddWithContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	m.mu.Lock()
	defer m.mu.Unlock()
	return syncStore(ctx, m.store, m.chain, index, m.syncBatchSize)
}

// IndexMode returns the index mode of the wallet manager.
func (m *Manager) IndexMode() IndexMode {
	return m.indexMode
}

// Close closes the wallet manager.
func (m *Manager) Close() error {
	m.tg.Stop()
	return nil
}

func syncStore(ctx context.Context, store Store, cm ChainManager, index types.ChainIndex, batchSize int) error {
	for index != cm.Tip() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		crus, caus, err := cm.UpdatesSince(index, batchSize)
		if err != nil {
			return fmt.Errorf("failed to subscribe to chain manager: %w", err)
		} else if err := store.UpdateChainState(crus, caus); err != nil {
			return fmt.Errorf("failed to update chain state: %w", err)
		}
		index = caus[len(caus)-1].State.Index
	}
	return nil
}

// NewManager creates a new wallet manager.
func NewManager(cm ChainManager, store Store, opts ...Option) (*Manager, error) {
	m := &Manager{
		indexMode:     IndexModePersonal,
		syncBatchSize: defaultSyncBatchSize,

		chain: cm,
		store: store,
		log:   zap.NewNop(),
		tg:    threadgroup.New(),
	}

	for _, opt := range opts {
		opt(m)
	}

	// if the index mode is none, skip setting the index mode in the store
	// and return the manager
	if m.indexMode == IndexModeNone {
		return m, nil
	} else if err := store.SetIndexMode(m.indexMode); err != nil {
		return nil, err
	}

	// start a goroutine to sync the store with the chain manager
	reorgChan := make(chan struct{}, 1)
	reorgChan <- struct{}{}
	unsubscribe := cm.OnReorg(func(index types.ChainIndex) {
		select {
		case reorgChan <- struct{}{}:
		default:
		}
	})

	go func() {
		defer unsubscribe()

		log := m.log.Named("sync")
		ctx, cancel, err := m.tg.AddWithContext(context.Background())
		if err != nil {
			log.Panic("failed to add to threadgroup", zap.Error(err))
		}
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case <-reorgChan:
			}

			m.mu.Lock()
			// update the store
			lastTip, err := store.LastCommittedIndex()
			if err != nil {
				log.Panic("failed to get last committed index", zap.Error(err))
			} else if err := syncStore(ctx, store, cm, lastTip, m.syncBatchSize); err != nil && !errors.Is(err, context.Canceled) {
				log.Panic("failed to sync store", zap.Error(err))
			}
			m.mu.Unlock()
		}
	}()
	return m, nil
}
