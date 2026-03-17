package blacklistcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
	"sync"

	elaCrypto "github.com/elastos/Elastos.ELA/crypto"
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
	nonce, err := GetAddBlacklistVoteNonce(o.contract, o.voterPubKey)
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

// SubmitBlacklistVotesBatch 批量提交添加黑名单投票：只拉取一次 nonce，然后依次用 nonce、nonce+1、nonce+2… 签名发送，保证多笔都能成功。
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
	nonce, err := GetAddBlacklistVoteNonce(o.contract, o.voterPubKey)
	if err != nil {
		return err
	}
	contractAddr := common.HexToAddress(o.contract)
	for i := range producerKeys {
		targetPubKey := common.Hex2Bytes(producerKeys[i])
		if len(targetPubKey) == 0 {
			log.Error("Invalid producer public key in batch", "producer", producerKeys[i], "index", i)
			continue
		}
		// 使用 nonce + i，保证每笔签名使用的 nonce 递增，链上执行时与合约内 nonce 一致
		ni := new(big.Int).Add(nonce, big.NewInt(int64(i)))
		message := buildAddBlacklistVoteMessage(contractAddr, chainID, targetPubKey, lastSealHeights[i], ni)
		signature := o.signer(message)
		if len(signature) == 0 {
			log.Error("Empty blacklist vote signature in batch", "producer", producerKeys[i], "index", i)
			continue
		}
		// 签名验证：校验签名是否为 voterPubKey 对应私钥、数据为 message
		valid := false
		if len(signature) > 0 && len(o.voterPubKey) == 33 {
			publicKey, err := elaCrypto.DecodePoint(o.voterPubKey)
			if err != nil {
				log.Error("Failed to decode voter public key when verifying signature in batch", "err", err)
			} else {
				err = elaCrypto.Verify(*publicKey, message, signature)
				if err != nil {
					log.Error("Failed to verify signature in batch", "err", err)
				} else {
					valid = true
				}
			}
		}
		if !valid {
			log.Error("Blacklist vote signature verification failed in batch", "producer", producerKeys[i], "index", i)
			continue
		}
		fmt.Println("targetPubKey", common.Bytes2Hex(targetPubKey))
		fmt.Println("voterPubKey", common.Bytes2Hex(o.voterPubKey))
		fmt.Println("valid", valid)

		digest := sha256.Sum256(message)
		fmt.Println("digest", common.Bytes2Hex(digest[:]))
		fmt.Println("signature", common.Bytes2Hex(signature))
		fmt.Println("message", common.Bytes2Hex(message))
		publicKey, err := elaCrypto.DecodePoint(o.voterPubKey)
		if err != nil {
			log.Error("Failed to decode voter public key when verifying signature in batch", "err", err)
		} else {
			err = elaCrypto.VerifyDigest(*publicKey, digest[:], signature)
			if err != nil {
				log.Error("Failed to verify signature in batch", "err", err)
			} else {
				fmt.Println("valid2", valid)
			}
		}
		txHash, err := SendBlacklistVote(o.contract, targetPubKey, lastSealHeights[i], o.voterPubKey, signature)
		if err != nil {
			log.Error("Submit blacklist vote failed in batch", "producer", producerKeys[i], "index", i, "error", err)
			continue
		}
		log.Info("Submit blacklist vote (batch)",
			"producer", producerKeys[i],
			"lastSealHeight", lastSealHeights[i],
			"txHash", txHash.String())
	}
	return nil
}

// RemoveBlacklistVote sends a blacklist removal vote to the contract.
func (o *ContractBlacklistOracle) RemoveBlacklistVote(producerKey string) error {
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
	nonce, err := GetRemoveBlacklistVoteNonce(o.contract, o.voterPubKey)
	if err != nil {
		return err
	}
	contractAddr := common.HexToAddress(o.contract)
	message := buildRemoveBlacklistVoteMessage(contractAddr, chainID, targetPubKey, nonce)
	signature := o.signer(message)
	if len(signature) == 0 {
		return fmt.Errorf("empty blacklist removal vote signature")
	}
	txHash, err := SendRemoveBlacklistVote(o.contract, targetPubKey, o.voterPubKey, signature)
	if err != nil {
		return err
	}
	log.Info("Submit remove blacklist vote",
		"producer", producerKey,
		"txHash", txHash.String())
	return nil
}

// SubmitRemoveBlacklistVotesBatch 批量提交移除黑名单投票：只拉取一次 nonce，然后依次用 nonce、nonce+1、nonce+2… 签名发送。
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
	nonce, err := GetRemoveBlacklistVoteNonce(o.contract, o.voterPubKey)
	if err != nil {
		return err
	}
	contractAddr := common.HexToAddress(o.contract)
	for i := range producerKeys {
		targetPubKey := common.Hex2Bytes(producerKeys[i])
		if len(targetPubKey) == 0 {
			log.Error("Invalid producer public key in remove batch", "producer", producerKeys[i], "index", i)
			continue
		}
		ni := new(big.Int).Add(nonce, big.NewInt(int64(i)))
		message := buildRemoveBlacklistVoteMessage(contractAddr, chainID, targetPubKey, ni)
		signature := o.signer(message)
		if len(signature) == 0 {
			log.Error("Empty remove blacklist vote signature in batch", "producer", producerKeys[i], "index", i)
			continue
		}
		txHash, err := SendRemoveBlacklistVote(o.contract, targetPubKey, o.voterPubKey, signature)
		if err != nil {
			log.Error("Submit remove blacklist vote failed in batch", "producer", producerKeys[i], "index", i, "error", err)
			continue
		}
		log.Info("Submit remove blacklist vote (batch)",
			"producer", producerKeys[i],
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
