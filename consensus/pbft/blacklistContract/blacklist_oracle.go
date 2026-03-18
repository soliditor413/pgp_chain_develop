package blacklistcontract

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"sync"

	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/log"
)

// ContractBlacklistOracle submits blacklist votes to a smart contract.
type ContractBlacklistOracle struct {
	contract    string
	voterPubKey []byte
	signer      func([]byte) []byte
	voteMu      sync.Mutex
	listenerMu  sync.Mutex
	listener    *BlacklistEventListener
}

// NewContractBlacklistOracle creates a blacklist oracle for the blacklist contract.
func NewContractBlacklistOracle(contract string, voterPubKey []byte, signer func([]byte) []byte) *ContractBlacklistOracle {
	return &ContractBlacklistOracle{
		contract:    contract,
		voterPubKey: append([]byte(nil), voterPubKey...),
		signer:      signer,
	}
}

// SubmitBlacklistVote sends a blacklist vote to the contract.
func (o *ContractBlacklistOracle) SubmitBlacklistVote(producerKey string, lastSealBlockHeight uint64) error {
	if o == nil {
		return nil
	}
	if o.contract == "" {
		return nil
	}
	if !common.IsHexAddress(o.contract) {
		return fmt.Errorf("blacklist contract address is invalid: %s", o.contract)
	}
	if o.signer == nil || len(o.voterPubKey) == 0 {
		return fmt.Errorf("blacklist signer is not configured")
	}
	targetPubKey := common.Hex2Bytes(producerKey)
	if len(targetPubKey) == 0 {
		return fmt.Errorf("invalid producer public key: %s", producerKey)
	}
	o.voteMu.Lock()
	defer o.voteMu.Unlock()
	chainID, err := GetChainID()
	if err != nil {
		return err
	}
	nonce, err := GetAddBlacklistVoteNonce(o.contract, o.voterPubKey, targetPubKey)
	if err != nil {
		return err
	}
	contractAddr := common.HexToAddress(o.contract)
	message := buildAddBlacklistVoteMessage(contractAddr, chainID, targetPubKey, lastSealBlockHeight, nonce)
	signature := o.signer(message)
	if len(signature) == 0 {
		return fmt.Errorf("empty blacklist vote signature")
	}
	txHash, err := SendBlacklistVote(o.contract, targetPubKey, lastSealBlockHeight, o.voterPubKey, signature)
	if err != nil {
		return err
	}
	log.Info("Submit blacklist vote",
		"producer", producerKey,
		"lastSealHeight", lastSealBlockHeight,
		"txHash", txHash.String())
	return nil
}

// SubmitBlacklistVotesBatch 批量提交添加黑名单投票。
// 合约 nonce 现在是 per-(voter, target)，每个 target 独立查询各自的 nonce。
// 以太坊 tx nonce 拉取一次后依次递增，保证所有交易都能进入 tx pool。
func (o *ContractBlacklistOracle) SubmitBlacklistVotesBatch(producerKeys []string, lastSealHeights []uint64) error {
	if o == nil {
		return nil
	}
	if len(producerKeys) != len(lastSealHeights) || len(producerKeys) == 0 {
		return fmt.Errorf("invalid batch: producerKeys and lastSealHeights length must match and be non-empty")
	}
	if o.contract == "" || !common.IsHexAddress(o.contract) {
		return fmt.Errorf("blacklist contract address is invalid: %s", o.contract)
	}
	if o.signer == nil || len(o.voterPubKey) == 0 {
		return fmt.Errorf("blacklist signer is not configured")
	}
	o.voteMu.Lock()
	defer o.voteMu.Unlock()
	chainID, err := GetChainID()
	if err != nil {
		return err
	}
	txNonce, err := GetPendingTxNonce()
	if err != nil {
		return err
	}
	contractAddr := common.HexToAddress(o.contract)
	txIdx := uint64(0)
	for i := range producerKeys {
		targetPubKey := common.Hex2Bytes(producerKeys[i])
		if len(targetPubKey) == 0 {
			log.Error("Invalid producer public key in batch", "producer", producerKeys[i], "index", i)
			continue
		}
		nonce, err := GetAddBlacklistVoteNonce(o.contract, o.voterPubKey, targetPubKey)
		if err != nil {
			log.Error("GetAddBlacklistVoteNonce failed in batch", "producer", producerKeys[i], "index", i, "error", err)
			continue
		}
		message := buildAddBlacklistVoteMessage(contractAddr, chainID, targetPubKey, lastSealHeights[i], nonce)
		signature := o.signer(message)
		if len(signature) == 0 {
			log.Error("Empty blacklist vote signature in batch", "producer", producerKeys[i], "index", i)
			continue
		}
		currentTxNonce := txNonce + txIdx
		txHash, err := SendBlacklistVoteWithNonce(o.contract, targetPubKey, lastSealHeights[i], o.voterPubKey, signature, &currentTxNonce)
		if err != nil {
			log.Error("Submit blacklist vote failed in batch", "producer", producerKeys[i], "index", i, "txNonce", currentTxNonce, "error", err)
			continue
		}
		txIdx++
		log.Info("Submit blacklist vote (batch)",
			"producer", producerKeys[i],
			"lastSealHeight", lastSealHeights[i],
			"txNonce", currentTxNonce,
			"txHash", txHash.String())
	}
	return nil
}

// SubmitRemoveBlacklistVotesBatch 批量提交移除黑名单投票。
// 合约 nonce 现在是 per-(voter, target)，每个 target 独立查询各自的 nonce。
// 以太坊 tx nonce 拉取一次后依次递增，保证所有交易都能进入 tx pool。
func (o *ContractBlacklistOracle) SubmitRemoveBlacklistVotesBatch(producerKeys []string) error {
	if o == nil {
		return nil
	}
	if len(producerKeys) == 0 {
		return nil
	}
	if o.contract == "" || !common.IsHexAddress(o.contract) {
		return fmt.Errorf("blacklist contract address is invalid: %s", o.contract)
	}
	if o.signer == nil || len(o.voterPubKey) == 0 {
		return fmt.Errorf("blacklist signer is not configured")
	}
	o.voteMu.Lock()
	defer o.voteMu.Unlock()
	chainID, err := GetChainID()
	if err != nil {
		return err
	}
	txNonce, err := GetPendingTxNonce()
	if err != nil {
		return err
	}
	contractAddr := common.HexToAddress(o.contract)
	txIdx := uint64(0)
	for i := range producerKeys {
		targetPubKey := common.Hex2Bytes(producerKeys[i])
		if len(targetPubKey) == 0 {
			log.Error("Invalid producer public key in remove batch", "producer", producerKeys[i], "index", i)
			continue
		}
		nonce, err := GetRemoveBlacklistVoteNonce(o.contract, o.voterPubKey, targetPubKey)
		if err != nil {
			log.Error("GetRemoveBlacklistVoteNonce failed in batch", "producer", producerKeys[i], "index", i, "error", err)
			continue
		}
		message := buildRemoveBlacklistVoteMessage(contractAddr, chainID, targetPubKey, nonce)
		signature := o.signer(message)
		if len(signature) == 0 {
			log.Error("Empty remove blacklist vote signature in batch", "producer", producerKeys[i], "index", i)
			continue
		}
		currentTxNonce := txNonce + txIdx
		txHash, err := SendRemoveBlacklistVoteWithNonce(o.contract, targetPubKey, o.voterPubKey, signature, &currentTxNonce)
		if err != nil {
			log.Error("Submit remove blacklist vote failed in batch", "producer", producerKeys[i], "index", i, "txNonce", currentTxNonce, "error", err)
			continue
		}
		txIdx++
		log.Info("Submit remove blacklist vote (batch)",
			"producer", producerKeys[i],
			"txNonce", currentTxNonce,
			"txHash", txHash.String())
	}
	return nil
}

// IsBlacklisted checks blacklist status from the contract.
func (o *ContractBlacklistOracle) IsBlacklisted(dposPublicKey []byte) (bool, error) {
	if o == nil || o.contract == "" {
		return false, nil
	}
	return IsBlacklisted(o.contract, dposPublicKey)
}

// HasAddVoted checks whether current voter public key already submitted an add vote.
func (o *ContractBlacklistOracle) HasAddVoted(dposPublicKey []byte) (bool, error) {
	if o == nil || o.contract == "" {
		return false, nil
	}
	return HasAddVoted(o.contract, dposPublicKey, o.voterPubKey)
}

// HasRemoveVoted checks whether current voter public key already submitted a remove vote.
func (o *ContractBlacklistOracle) HasRemoveVoted(dposPublicKey []byte) (bool, error) {
	if o == nil || o.contract == "" {
		return false, nil
	}
	return HasRemoveVoted(o.contract, dposPublicKey, o.voterPubKey)
}

// IsExpired checks whether a blacklist entry is expired in the contract view.
func (o *ContractBlacklistOracle) IsExpired(dposPublicKey []byte) (bool, error) {
	if o == nil || o.contract == "" {
		return false, nil
	}
	return IsBlacklistExpired(o.contract, dposPublicKey)
}

// StartListener subscribes to blacklist contract events.
func (o *ContractBlacklistOracle) StartListener(onConfirmed func([]byte), onRemoved func([]byte), getScannedHeight func() uint64, setScannedHeight func(uint64)) error {
	if o == nil || o.contract == "" {
		return nil
	}
	o.listenerMu.Lock()
	defer o.listenerMu.Unlock()

	if o.listener != nil {
		o.listener.Stop()
	}
	listener := NewBlacklistEventListener(o.contract, onConfirmed, onRemoved, getScannedHeight, setScannedHeight)
	if err := listener.Start(); err != nil {
		return err
	}
	o.listener = listener
	return nil
}

// StopListener stops the blacklist contract event subscription.
func (o *ContractBlacklistOracle) StopListener() {
	if o == nil {
		return
	}
	o.listenerMu.Lock()
	defer o.listenerMu.Unlock()

	if o.listener != nil {
		o.listener.Stop()
		o.listener = nil
	}
}

func buildAddBlacklistVoteMessage(contractAddr common.Address, chainID *big.Int, dposPublicKey []byte, lastSealBlockHeight uint64, nonce *big.Int) []byte {
	var buf bytes.Buffer
	buf.Write(contractAddr.Bytes())
	buf.Write(encodeUint256(chainID))
	buf.WriteByte(1)
	buf.Write(dposPublicKey)
	height := make([]byte, 8)
	binary.BigEndian.PutUint64(height, lastSealBlockHeight)
	buf.Write(height)
	buf.Write(encodeUint256(nonce))
	return buf.Bytes()
}

func buildRemoveBlacklistVoteMessage(contractAddr common.Address, chainID *big.Int, dposPublicKey []byte, nonce *big.Int) []byte {
	var buf bytes.Buffer
	buf.Write(contractAddr.Bytes())
	buf.Write(encodeUint256(chainID))
	buf.WriteByte(2)
	buf.Write(dposPublicKey)
	buf.Write(encodeUint256(nonce))
	return buf.Bytes()
}

func encodeUint256(value *big.Int) []byte {
	if value == nil {
		return make([]byte, 32)
	}
	b := value.Bytes()
	if len(b) > 32 {
		return b[len(b)-32:]
	}
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return padded
}
