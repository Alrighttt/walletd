package wallet

import (
	"time"

	"go.sia.tech/core/types"
)

// event types indicate the source of an event. Events can
// either be created by sending Siacoins between addresses or they can be
// created by consensus (e.g. a miner payout, a siafund claim, or a contract).
const (
	EventTypeMinerPayout       = "miner"
	EventTypeFoundationSubsidy = "foundation"

	EventTypeV1Transaction        = "v1Transaction"
	EventTypeV1ContractResolution = "v1ContractResolution"

	EventTypeV2Transaction        = "v2Transaction"
	EventTypeV2ContractResolution = "v2ContractResolution"

	EventTypeSiafundClaim = "siafundClaim"
)

type (
	EventData interface {
		isEvent() bool
	}

	// An Event is something interesting that happened on the Sia blockchain.
	Event struct {
		ID             types.Hash256    `json:"id"`
		Index          types.ChainIndex `json:"index"`
		Timestamp      time.Time        `json:"timestamp"`
		MaturityHeight uint64           `json:"maturityHeight"`
		Type           string           `json:"type"`
		Data           EventData        `json:"data"`
		Relevant       []types.Address  `json:"relevant,omitempty"`
	}

	EventV1Transaction struct {
		Transaction types.Transaction `json:"transaction"`
		// v1 siacoin inputs do not describe the value of the spent utxo
		SpentSiacoinElements []types.SiacoinElement `json:"spentSiacoinElements"`
		// v1 siafund inputs do not describe the value of the spent utxo
		SpentSiafundElements []types.SiafundElement `json:"spentSiafundElements"`
	}

	EventV2Transaction types.V2Transaction

	// An EventPayout represents a payout from a siafund claim, a miner, or the
	// foundation subsidy.
	EventPayout struct {
		SiacoinElement types.SiacoinElement `json:"siacoinElement"`
	}

	// An EventV1ContractResolution represents a file contract payout from a v1
	// contract.
	EventV1ContractResolution struct {
		FileContract   types.FileContractElement `json:"fileContract"`
		SiacoinElement types.SiacoinElement      `json:"siacoinElement"`
		Missed         bool                      `json:"missed"`
	}

	// An EventV2ContractResolution represents a file contract payout from a v2
	// contract.
	EventV2ContractResolution struct {
		FileContract   types.V2FileContractElement        `json:"fileContract"`
		Resolution     types.V2FileContractResolutionType `json:"resolution"`
		SiacoinElement types.SiacoinElement               `json:"siacoinElement"`
		Missed         bool                               `json:"missed"`
	}
)

func (EventPayout) isEvent() bool               { return true }
func (EventV1ContractResolution) isEvent() bool { return true }
func (EventV2ContractResolution) isEvent() bool { return true }
func (EventV1Transaction) isEvent() bool        { return true }
func (EventV2Transaction) isEvent() bool        { return true }
