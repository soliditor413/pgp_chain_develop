package blacklistcontract

import (
	"math/big"

	"github.com/pgprotocol/pgp-chain/common"
	ctypes "github.com/pgprotocol/pgp-chain/core/types"
)

// BlacklistEvent represents a parsed blacklist contract event.
type BlacklistEvent struct {
	DposPublicKey []byte
	IsConfirmed   bool // true = BlacklistConfirmed, false = BlacklistRemoved
	LogRemoved    bool // true if the log itself was reverted (chain reorg)
}

// ParseBlacklistEvents extracts BlacklistConfirmed and BlacklistRemoved events
// from the given receipts for the specified blacklist contract address.
// This is meant to be called synchronously per block during chain processing,
// so that blacklist state stays in lockstep with the chain.
func ParseBlacklistEvents(contract string, receipts ctypes.Receipts) []BlacklistEvent {
	if !common.IsHexAddress(contract) || len(receipts) == 0 {
		return nil
	}
	contractAddr := common.HexToAddress(contract)
	confirmedID := blacklistABI.Events["BlacklistConfirmed"].ID
	removedID := blacklistABI.Events["BlacklistRemoved"].ID

	var events []BlacklistEvent
	for _, receipt := range receipts {
		for _, vLog := range receipt.Logs {
			if vLog.Address != contractAddr || len(vLog.Topics) == 0 {
				continue
			}
			switch vLog.Topics[0] {
			case confirmedID:
				key, err := decodeConfirmedProducerKey(vLog.Data)
				if err != nil {
					continue
				}
				events = append(events, BlacklistEvent{
					DposPublicKey: key,
					IsConfirmed:   true,
					LogRemoved:    vLog.Removed,
				})
			case removedID:
				key, err := decodeRemovedProducerKey(vLog.Data)
				if err != nil {
					continue
				}
				events = append(events, BlacklistEvent{
					DposPublicKey: key,
					IsConfirmed:   false,
					LogRemoved:    vLog.Removed,
				})
			}
		}
	}
	return events
}

func decodeConfirmedProducerKey(data []byte) ([]byte, error) {
	var event struct {
		DposPublicKey []byte   `abi:"dposPublicKey"`
		Votes         *big.Int `abi:"votes"`
	}
	if err := blacklistABI.UnpackIntoInterface(&event, "BlacklistConfirmed", data); err != nil {
		return nil, err
	}
	return append([]byte(nil), event.DposPublicKey...), nil
}

func decodeRemovedProducerKey(data []byte) ([]byte, error) {
	var event struct {
		DposPublicKey []byte `abi:"dposPublicKey"`
	}
	if err := blacklistABI.UnpackIntoInterface(&event, "BlacklistRemoved", data); err != nil {
		return nil, err
	}
	return append([]byte(nil), event.DposPublicKey...), nil
}
