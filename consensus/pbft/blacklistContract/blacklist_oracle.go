package blacklistcontract

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/log"
)

// ContractBlacklistOracle submits blacklist votes to a smart contract.
type ContractBlacklistOracle struct {
	contract    string
	voterPubKey []byte
	signer      func([]byte) []byte
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
	chainID, err := GetChainID()
	if err != nil {
		return err
	}
	nonce, err := GetBlacklistVoteNonce(o.contract, o.voterPubKey)
	if err != nil {
		return err
	}
	contractAddr := common.HexToAddress(o.contract)
	message := buildBlacklistVoteMessage(contractAddr, chainID, targetPubKey, lastSealBlockHeight, nonce)
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

// RemoveBlacklistVote sends a blacklist removal vote to the contract.
func (o *ContractBlacklistOracle) RemoveBlacklistVote(producerKey string, lastSealBlockHeight uint64) error {
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
	blacklisted, err := IsBlacklisted(o.contract, targetPubKey)
	if err != nil {
		return err
	}
	if !blacklisted {
		return nil
	}
	chainID, err := GetChainID()
	if err != nil {
		return err
	}
	nonce, err := GetBlacklistVoteNonce(o.contract, o.voterPubKey)
	if err != nil {
		return err
	}
	contractAddr := common.HexToAddress(o.contract)
	message := buildBlacklistVoteMessage(contractAddr, chainID, targetPubKey, lastSealBlockHeight, nonce)
	signature := o.signer(message)
	if len(signature) == 0 {
		return fmt.Errorf("empty blacklist removal vote signature")
	}
	txHash, err := SendRemoveBlacklistVote(o.contract, targetPubKey, lastSealBlockHeight, o.voterPubKey, signature)
	if err != nil {
		return err
	}
	log.Info("Submit remove blacklist vote",
		"producer", producerKey,
		"lastSealHeight", lastSealBlockHeight,
		"txHash", txHash.String())
	return nil
}

// IsBlacklisted checks blacklist status from the contract.
func (o *ContractBlacklistOracle) IsBlacklisted(dposPublicKey []byte) (bool, error) {
	if o == nil || o.contract == "" {
		return false, nil
	}
	return IsBlacklisted(o.contract, dposPublicKey)
}

func buildBlacklistVoteMessage(contractAddr common.Address, chainID *big.Int, dposPublicKey []byte, lastSealBlockHeight uint64, nonce *big.Int) []byte {
	var buf bytes.Buffer
	buf.Write(contractAddr.Bytes())
	buf.Write(encodeUint256(chainID))
	buf.Write(dposPublicKey)
	height := make([]byte, 8)
	binary.BigEndian.PutUint64(height, lastSealBlockHeight)
	buf.Write(height)
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
