package blacklistcontract

import (
	"context"
	"fmt"
	"math/big"
	"time"

	ethereum "github.com/pgprotocol/pgp-chain"
	"github.com/pgprotocol/pgp-chain/common"
	ctypes "github.com/pgprotocol/pgp-chain/core/types"
	"github.com/pgprotocol/pgp-chain/log"
	"github.com/pgprotocol/pgp-chain/spv"
)

const blacklistListenerRetryInterval = 3 * time.Second

// BlacklistEventListener watches blacklist contract events and resolves producer keys from tx input.
type BlacklistEventListener struct {
	contract         string
	onConfirmed      func([]byte)
	onRemoved        func([]byte)
	getScannedHeight func() uint64
	setScannedHeight func(uint64)
	cancel           context.CancelFunc
	done             chan struct{}
}

// NewBlacklistEventListener creates a listener for blacklist contract events.
func NewBlacklistEventListener(contract string, onConfirmed func([]byte), onRemoved func([]byte), getScannedHeight func() uint64, setScannedHeight func(uint64)) *BlacklistEventListener {
	return &BlacklistEventListener{
		contract:         contract,
		onConfirmed:      onConfirmed,
		onRemoved:        onRemoved,
		getScannedHeight: getScannedHeight,
		setScannedHeight: setScannedHeight,
	}
}

// Start begins the background log subscription.
func (l *BlacklistEventListener) Start() error {
	if l == nil || l.contract == "" {
		return nil
	}
	if !common.IsHexAddress(l.contract) {
		return fmt.Errorf("invalid blacklist contract address: %s", l.contract)
	}
	if l.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	l.done = make(chan struct{})
	go l.loop(ctx)
	return nil
}

// Stop terminates the log subscription.
func (l *BlacklistEventListener) Stop() {
	if l == nil || l.cancel == nil {
		return
	}
	l.cancel()
	if l.done != nil {
		<-l.done
	}
	l.cancel = nil
	l.done = nil
}

func (l *BlacklistEventListener) loop(ctx context.Context) {
	defer close(l.done)

	contractAddr := common.HexToAddress(l.contract)
	query := ethereum.FilterQuery{
		Addresses: []common.Address{contractAddr},
		Topics: [][]common.Hash{{
			blacklistABI.Events["BlacklistConfirmed"].ID,
			blacklistABI.Events["BlacklistRemoved"].ID,
		}},
	}

	for {
		if ctx.Err() != nil {
			return
		}
		client := spv.GetIPCClient()
		if client == nil {
			log.Warn("Blacklist listener waiting for ipc client")
			if !waitForBlacklistRetry(ctx) {
				return
			}
			continue
		}

		latest, err := client.CurrentBlockNumber(ctx)
		if err != nil {
			log.Warn("Blacklist listener failed to get current block number", "error", err)
			if !waitForBlacklistRetry(ctx) {
				return
			}
			continue
		}
		start := l.nextStartHeight()
		if start <= latest {
			if err := l.catchUpLogs(ctx, client, query, start, latest); err != nil {
				log.Warn("Blacklist listener catch-up failed", "from", start, "to", latest, "error", err)
				if !waitForBlacklistRetry(ctx) {
					return
				}
				continue
			}
		}

		logCh := make(chan ctypes.Log, 16)
		liveQuery := query
		liveQuery.FromBlock = big.NewInt(0).SetUint64(latest + 1)
		sub, err := client.SubscribeFilterLogs(ctx, liveQuery, logCh)
		if err != nil {
			log.Warn("Blacklist listener subscribe failed", "error", err)
			if !waitForBlacklistRetry(ctx) {
				return
			}
			continue
		}

		err = l.consume(ctx, sub, logCh)
		sub.Unsubscribe()
		if err == nil || ctx.Err() != nil {
			return
		}
		log.Warn("Blacklist listener resubscribing", "error", err)
		if !waitForBlacklistRetry(ctx) {
			return
		}
	}
}

func (l *BlacklistEventListener) catchUpLogs(ctx context.Context, client interface {
	FilterLogs(context.Context, ethereum.FilterQuery) ([]ctypes.Log, error)
}, query ethereum.FilterQuery, from, to uint64) error {
	catchUpQuery := query
	catchUpQuery.FromBlock = big.NewInt(0).SetUint64(from)
	catchUpQuery.ToBlock = big.NewInt(0).SetUint64(to)

	logs, err := client.FilterLogs(ctx, catchUpQuery)
	if err != nil {
		return err
	}
	for _, vLog := range logs {
		l.handleLog(vLog)
		l.markScannedHeight(vLog.BlockNumber)
	}
	l.markScannedHeight(to)
	return nil
}

func (l *BlacklistEventListener) consume(ctx context.Context, sub ethereum.Subscription, logCh <-chan ctypes.Log) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-sub.Err():
			if !ok {
				return nil
			}
			return err
		case vLog, ok := <-logCh:
			if !ok {
				return nil
			}
			l.handleLog(vLog)
			l.markScannedHeight(vLog.BlockNumber)
		}
	}
}

func (l *BlacklistEventListener) handleLog(vLog ctypes.Log) {
	if len(vLog.Topics) == 0 {
		return
	}

	switch vLog.Topics[0] {
	case blacklistABI.Events["BlacklistConfirmed"].ID:
		dposPublicKey, err := decodeConfirmedProducerKey(vLog.Data)
		if err != nil {
			log.Warn("Blacklist listener failed to decode confirmed producer key", "txHash", vLog.TxHash, "error", err)
			return
		}
		if vLog.Removed {
			if l.onRemoved != nil {
				l.onRemoved(dposPublicKey)
			}
			return
		}
		if l.onConfirmed != nil {
			l.onConfirmed(dposPublicKey)
		}
	case blacklistABI.Events["BlacklistRemoved"].ID:
		dposPublicKey, err := decodeRemovedProducerKey(vLog.Data)
		if err != nil {
			log.Warn("Blacklist listener failed to decode removed producer key", "txHash", vLog.TxHash, "error", err)
			return
		}
		if vLog.Removed {
			blacklisted, err := IsBlacklisted(l.contract, dposPublicKey)
			if err != nil {
				log.Warn("Blacklist listener failed to reconcile removed log", "txHash", vLog.TxHash, "error", err)
				return
			}
			if blacklisted {
				if l.onConfirmed != nil {
					l.onConfirmed(dposPublicKey)
				}
				return
			}
		}
		if l.onRemoved != nil {
			l.onRemoved(dposPublicKey)
		}
	}
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

func waitForBlacklistRetry(ctx context.Context) bool {
	timer := time.NewTimer(blacklistListenerRetryInterval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (l *BlacklistEventListener) nextStartHeight() uint64 {
	if l == nil || l.getScannedHeight == nil {
		return 0
	}
	scanned := l.getScannedHeight()
	if scanned == ^uint64(0) {
		return scanned
	}
	return scanned + 1
}

func (l *BlacklistEventListener) markScannedHeight(height uint64) {
	if l == nil || l.setScannedHeight == nil {
		return
	}
	l.setScannedHeight(height)
}
