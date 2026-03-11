package blacklistcontract

import (
	"context"
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/pgprotocol/pgp-chain"
	"github.com/pgprotocol/pgp-chain/accounts/abi"
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/ethclient"
	"github.com/pgprotocol/pgp-chain/log"
	"github.com/pgprotocol/pgp-chain/spv"
)

const blacklistABIMetaData = `[
	{
		"inputs": [
			{
				"internalType": "bytes",
				"name": "dposPublicKey",
				"type": "bytes"
			},
			{
				"internalType": "uint64",
				"name": "lastSealBlockHeight",
				"type": "uint64"
			},
			{
				"internalType": "bytes",
				"name": "voterPublicKey",
				"type": "bytes"
			},
			{
				"internalType": "bytes",
				"name": "signature",
				"type": "bytes"
			}
		],
		"name": "addBlacklistVote",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "bytes",
				"name": "dposPublicKey",
				"type": "bytes"
			},
			{
				"internalType": "uint64",
				"name": "lastSealBlockHeight",
				"type": "uint64"
			},
			{
				"internalType": "bytes",
				"name": "voterPublicKey",
				"type": "bytes"
			},
			{
				"internalType": "bytes",
				"name": "signature",
				"type": "bytes"
			}
		],
		"name": "removeBlacklistVote",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "bytes",
				"name": "voterPublicKey",
				"type": "bytes"
			}
		],
		"name": "getVoteNonce",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "nonce",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "bytes",
				"name": "dposPublicKey",
				"type": "bytes"
			}
		],
		"name": "isBlacklisted",
		"outputs": [
			{
				"internalType": "bool",
				"name": "",
				"type": "bool"
			}
		],
		"stateMutability": "view",
		"type": "function"
	}
]`

var blacklistABI abi.ABI

func init() {
	parsed, err := abi.JSON(strings.NewReader(blacklistABIMetaData))
	if err != nil {
		panic(err)
	}
	blacklistABI = parsed
}

// SendBlacklistVote submits a blacklist vote transaction to the configured contract.
func SendBlacklistVote(contract string, dposPublicKey []byte, lastSealBlockHeight uint64, voterPublicKey []byte, signature []byte) (common.Hash, error) {
	client := spv.GetIPCClient()
	if client == nil {
		return common.Hash{}, errors.New("spv ipc client is nil")
	}
	if !common.IsHexAddress(contract) {
		return common.Hash{}, errors.New("invalid blacklist contract address")
	}
	if spv.GetDefaultSingerAddr == nil {
		return common.Hash{}, errors.New("default signer address is not configured")
	}
	from := spv.GetDefaultSingerAddr()
	if (from == common.Address{}) {
		return common.Hash{}, errors.New("default signer address is empty")
	}
	if len(dposPublicKey) == 0 || len(voterPublicKey) == 0 || len(signature) == 0 {
		return common.Hash{}, errors.New("invalid blacklist vote parameters")
	}

	inputData, err := blacklistABI.Pack("addBlacklistVote", dposPublicKey, lastSealBlockHeight, voterPublicKey, signature)
	if err != nil {
		return common.Hash{}, err
	}
	contractAddr := common.HexToAddress(contract)
	if err := precheckContractCall(client, from, contractAddr, inputData); err != nil {
		log.Error("Blacklist vote PreCheck ContractCall failed", "error", err)
		return common.Hash{}, err
	}
	msg := ethereum.CallMsg{From: from, To: &contractAddr, Data: inputData}
	gasLimit, err := client.EstimateGas(context.Background(), msg)
	if err != nil {
		log.Error("Blacklist vote EstimateGas failed", "error", err)
		return common.Hash{}, err
	}
	if gasLimit == 0 {
		return common.Hash{}, errors.New("blacklist vote EstimateGas is 0")
	}
	price, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Error("Blacklist vote SuggestGasPrice failed", "error", err)
		return common.Hash{}, err
	}
	callmsg := ethereum.TXMsg{
		From:     from,
		To:       &contractAddr,
		Gas:      gasLimit,
		Data:     inputData,
		GasPrice: price,
	}
	return client.SendPublicTransaction(context.Background(), callmsg)
}

// SendRemoveBlacklistVote submits a blacklist removal vote transaction.
func SendRemoveBlacklistVote(contract string, dposPublicKey []byte, lastSealBlockHeight uint64, voterPublicKey []byte, signature []byte) (common.Hash, error) {
	client := spv.GetIPCClient()
	if client == nil {
		return common.Hash{}, errors.New("spv ipc client is nil")
	}
	if !common.IsHexAddress(contract) {
		return common.Hash{}, errors.New("invalid blacklist contract address")
	}
	if spv.GetDefaultSingerAddr == nil {
		return common.Hash{}, errors.New("default signer address is not configured")
	}
	from := spv.GetDefaultSingerAddr()
	if (from == common.Address{}) {
		return common.Hash{}, errors.New("default signer address is empty")
	}
	if len(dposPublicKey) == 0 || len(voterPublicKey) == 0 || len(signature) == 0 {
		return common.Hash{}, errors.New("invalid remove blacklist vote parameters")
	}

	inputData, err := blacklistABI.Pack("removeBlacklistVote", dposPublicKey, lastSealBlockHeight, voterPublicKey, signature)
	if err != nil {
		return common.Hash{}, err
	}
	contractAddr := common.HexToAddress(contract)
	if err := precheckContractCall(client, from, contractAddr, inputData); err != nil {
		return common.Hash{}, err
	}
	msg := ethereum.CallMsg{From: from, To: &contractAddr, Data: inputData}
	gasLimit, err := client.EstimateGas(context.Background(), msg)
	if err != nil {
		log.Error("Remove blacklist vote EstimateGas failed", "error", err)
		return common.Hash{}, err
	}
	if gasLimit == 0 {
		return common.Hash{}, errors.New("remove blacklist vote EstimateGas is 0")
	}
	callmsg := ethereum.TXMsg{
		From:     from,
		To:       &contractAddr,
		Gas:      gasLimit,
		Data:     inputData,
		GasPrice: big.NewInt(0),
	}
	return client.SendPublicTransaction(context.Background(), callmsg)
}

// GetBlacklistVoteNonce returns the current vote nonce for a voter public key.
func GetBlacklistVoteNonce(contract string, voterPublicKey []byte) (*big.Int, error) {
	client := spv.GetIPCClient()
	if client == nil {
		return nil, errors.New("spv ipc client is nil")
	}
	if !common.IsHexAddress(contract) {
		return nil, errors.New("invalid blacklist contract address")
	}
	if len(voterPublicKey) == 0 {
		return nil, errors.New("voter public key is empty")
	}
	inputData, err := blacklistABI.Pack("getVoteNonce", voterPublicKey)
	if err != nil {
		return nil, err
	}
	contractAddr := common.HexToAddress(contract)
	msg := ethereum.CallMsg{From: common.Address{}, To: &contractAddr, Data: inputData}
	out, err := client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return nil, err
	}
	values, err := blacklistABI.Unpack("getVoteNonce", out)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, errors.New("invalid getVoteNonce response")
	}
	nonce, ok := values[0].(*big.Int)
	if !ok {
		return nil, errors.New("invalid nonce type")
	}
	return nonce, nil
}

// IsBlacklisted queries blacklist status from the contract.
func IsBlacklisted(contract string, dposPublicKey []byte) (bool, error) {
	client := spv.GetIPCClient()
	if client == nil {
		return false, errors.New("spv ipc client is nil")
	}
	if !common.IsHexAddress(contract) {
		return false, errors.New("invalid blacklist contract address")
	}
	if len(dposPublicKey) == 0 {
		return false, errors.New("dpos public key is empty")
	}
	inputData, err := blacklistABI.Pack("isBlacklisted", dposPublicKey)
	if err != nil {
		return false, err
	}
	contractAddr := common.HexToAddress(contract)
	msg := ethereum.CallMsg{From: common.Address{}, To: &contractAddr, Data: inputData}
	out, err := client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return false, err
	}
	values, err := blacklistABI.Unpack("isBlacklisted", out)
	if err != nil {
		return false, err
	}
	if len(values) != 1 {
		return false, errors.New("invalid isBlacklisted response")
	}
	blacklisted, ok := values[0].(bool)
	if !ok {
		return false, errors.New("invalid isBlacklisted type")
	}
	return blacklisted, nil
}

// GetChainID returns the chain id from the ipc client.
func GetChainID() (*big.Int, error) {
	client := spv.GetIPCClient()
	if client == nil {
		return nil, errors.New("spv ipc client is nil")
	}
	return client.ChainID(context.Background())
}

func precheckContractCall(client *ethclient.Client, from common.Address, contractAddr common.Address, inputData []byte) error {
	msg := ethereum.CallMsg{From: from, To: &contractAddr, Data: inputData}
	_, err := client.CallContract(context.Background(), msg, nil)
	if err != nil {
		log.Warn("Blacklist vote precheck failed", "error", err)
		return err
	}
	return nil
}
