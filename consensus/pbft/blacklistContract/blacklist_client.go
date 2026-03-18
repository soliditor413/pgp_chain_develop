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

const (
	blacklistVoteFallbackGasLimit uint64 = 500000
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
			},
			{
				"internalType": "bytes",
				"name": "targetPublicKey",
				"type": "bytes"
			}
		],
		"name": "getAddVoteNonce",
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
				"name": "voterPublicKey",
				"type": "bytes"
			},
			{
				"internalType": "bytes",
				"name": "targetPublicKey",
				"type": "bytes"
			}
		],
		"name": "getRemoveVoteNonce",
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
	},
	{
		"inputs": [
			{
				"internalType": "bytes",
				"name": "dposPublicKey",
				"type": "bytes"
			},
			{
				"internalType": "bytes",
				"name": "voterPublicKey",
				"type": "bytes"
			}
		],
		"name": "hasAddVoted",
		"outputs": [
			{
				"internalType": "bool",
				"name": "voted",
				"type": "bool"
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
			},
			{
				"internalType": "bytes",
				"name": "voterPublicKey",
				"type": "bytes"
			}
		],
		"name": "hasRemoveVoted",
		"outputs": [
			{
				"internalType": "bool",
				"name": "voted",
				"type": "bool"
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
		"name": "getBlacklistEntry",
		"outputs": [
			{
				"components": [
					{
						"internalType": "bytes",
						"name": "dposPublicKey",
						"type": "bytes"
					},
					{
						"internalType": "uint256",
						"name": "startedAtHeight",
						"type": "uint256"
					},
					{
						"internalType": "uint256",
						"name": "addedAtBlockHeight",
						"type": "uint256"
					},
					{
						"internalType": "uint256",
						"name": "lastSealBlockHeight",
						"type": "uint256"
					},
					{
						"internalType": "uint256",
						"name": "votes",
						"type": "uint256"
					},
					{
						"internalType": "enum IBlacklistManager.BlacklistStatus",
						"name": "status",
						"type": "uint8"
					}
				],
				"internalType": "struct IBlacklistManager.BlacklistEntry",
				"name": "entry",
				"type": "tuple"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": false,
				"internalType": "bytes",
				"name": "dposPublicKey",
				"type": "bytes"
			},
			{
				"indexed": false,
				"internalType": "uint256",
				"name": "votes",
				"type": "uint256"
			}
		],
		"name": "BlacklistConfirmed",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": false,
				"internalType": "bytes",
				"name": "dposPublicKey",
				"type": "bytes"
			}
		],
		"name": "BlacklistRemoved",
		"type": "event"
	}
]`

var blacklistABI abi.ABI

type BlacklistStatus uint8

const (
	BlacklistStatusNone BlacklistStatus = iota
	BlacklistStatusPending
	BlacklistStatusConfirmed
	BlacklistStatusExpired
)

type BlacklistEntry struct {
	DposPublicKey       []byte   `abi:"dposPublicKey"`
	StartedAtHeight     *big.Int `abi:"startedAtHeight"`
	AddedAtBlockHeight  *big.Int `abi:"addedAtBlockHeight"`
	LastSealBlockHeight *big.Int `abi:"lastSealBlockHeight"`
	Votes               *big.Int `abi:"votes"`
	Status              uint8    `abi:"status"`
}

func init() {
	parsed, err := abi.JSON(strings.NewReader(blacklistABIMetaData))
	if err != nil {
		panic(err)
	}
	blacklistABI = parsed
}

// SendBlacklistVote submits a blacklist vote transaction to the configured contract.
func SendBlacklistVote(contract string, dposPublicKey []byte, lastSealBlockHeight uint64, voterPublicKey []byte, signature []byte) (common.Hash, error) {
	return SendBlacklistVoteWithNonce(contract, dposPublicKey, lastSealBlockHeight, voterPublicKey, signature, nil)
}

// SendBlacklistVoteWithNonce submits a blacklist vote transaction with an explicit nonce.
// If nonceOverride is nil, it queries PendingNonceAt automatically.
func SendBlacklistVoteWithNonce(contract string, dposPublicKey []byte, lastSealBlockHeight uint64, voterPublicKey []byte, signature []byte, nonceOverride *uint64) (common.Hash, error) {
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
	gasLimit := uint64(800000)
	gasprice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return common.Hash{}, err
	}
	price := new(big.Int).Mul(gasprice, big.NewInt(100+10))
	price.Div(price, big.NewInt(100))
	var nonce uint64
	if nonceOverride != nil {
		nonce = *nonceOverride
	} else {
		nonce, err = client.PendingNonceAt(context.Background(), from)
		if err != nil {
			return common.Hash{}, err
		}
	}
	callMsg := ethereum.TXMsg{
		From:     from,
		To:       &contractAddr,
		Gas:      gasLimit,
		Data:     inputData,
		GasPrice: price,
		Nonce:    nonce,
	}
	return client.SendPublicTransaction(context.Background(), callMsg)
}

// SendRemoveBlacklistVote submits a blacklist removal vote transaction.
func SendRemoveBlacklistVote(contract string, dposPublicKey []byte, voterPublicKey []byte, signature []byte) (common.Hash, error) {
	return SendRemoveBlacklistVoteWithNonce(contract, dposPublicKey, voterPublicKey, signature, nil)
}

// SendRemoveBlacklistVoteWithNonce submits a blacklist removal vote transaction with an explicit nonce.
// If nonceOverride is nil, it queries PendingNonceAt automatically.
func SendRemoveBlacklistVoteWithNonce(contract string, dposPublicKey []byte, voterPublicKey []byte, signature []byte, nonceOverride *uint64) (common.Hash, error) {
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

	inputData, err := blacklistABI.Pack("removeBlacklistVote", dposPublicKey, voterPublicKey, signature)
	if err != nil {
		return common.Hash{}, err
	}
	contractAddr := common.HexToAddress(contract)
	if err := precheckContractCall(client, from, contractAddr, inputData); err != nil {
		return common.Hash{}, err
	}

	gasLimit := uint64(800000)
	gasprice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return common.Hash{}, err
	}
	price := new(big.Int).Mul(gasprice, big.NewInt(100+10))
	price.Div(price, big.NewInt(100))
	var nonce uint64
	if nonceOverride != nil {
		nonce = *nonceOverride
	} else {
		nonce, err = client.PendingNonceAt(context.Background(), from)
		if err != nil {
			return common.Hash{}, err
		}
	}
	callmsg := ethereum.TXMsg{
		From:     from,
		To:       &contractAddr,
		Gas:      gasLimit,
		Data:     inputData,
		GasPrice: price,
		Nonce:    nonce,
	}
	return client.SendPublicTransaction(context.Background(), callmsg)
}

// GetPendingTxNonce returns the pending nonce for the default signer address.
func GetPendingTxNonce() (uint64, error) {
	client := spv.GetIPCClient()
	if client == nil {
		return 0, errors.New("spv ipc client is nil")
	}
	if spv.GetDefaultSingerAddr == nil {
		return 0, errors.New("default signer address is not configured")
	}
	from := spv.GetDefaultSingerAddr()
	if (from == common.Address{}) {
		return 0, errors.New("default signer address is empty")
	}
	return client.PendingNonceAt(context.Background(), from)
}

// GetAddBlacklistVoteNonce returns the add-vote nonce for a (voter, target) pair from the pending state.
func GetAddBlacklistVoteNonce(contract string, voterPublicKey []byte, targetPublicKey []byte) (*big.Int, error) {
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
	if len(targetPublicKey) == 0 {
		return nil, errors.New("target public key is empty")
	}
	inputData, err := blacklistABI.Pack("getAddVoteNonce", voterPublicKey, targetPublicKey)
	if err != nil {
		return nil, err
	}
	contractAddr := common.HexToAddress(contract)
	msg := ethereum.CallMsg{From: common.Address{}, To: &contractAddr, Data: inputData}
	out, err := client.PendingCallContract(context.Background(), msg)
	if err != nil {
		return nil, err
	}
	values, err := blacklistABI.Unpack("getAddVoteNonce", out)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, errors.New("invalid getAddVoteNonce response")
	}
	nonce, ok := values[0].(*big.Int)
	if !ok {
		return nil, errors.New("invalid add nonce type")
	}
	return nonce, nil
}

// GetRemoveBlacklistVoteNonce returns the remove-vote nonce for a (voter, target) pair from the pending state.
func GetRemoveBlacklistVoteNonce(contract string, voterPublicKey []byte, targetPublicKey []byte) (*big.Int, error) {
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
	if len(targetPublicKey) == 0 {
		return nil, errors.New("target public key is empty")
	}
	inputData, err := blacklistABI.Pack("getRemoveVoteNonce", voterPublicKey, targetPublicKey)
	if err != nil {
		return nil, err
	}
	contractAddr := common.HexToAddress(contract)
	msg := ethereum.CallMsg{From: common.Address{}, To: &contractAddr, Data: inputData}
	out, err := client.PendingCallContract(context.Background(), msg)
	if err != nil {
		return nil, err
	}
	values, err := blacklistABI.Unpack("getRemoveVoteNonce", out)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, errors.New("invalid getRemoveVoteNonce response")
	}
	nonce, ok := values[0].(*big.Int)
	if !ok {
		return nil, errors.New("invalid remove nonce type")
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

// HasAddVoted checks whether voterPublicKey already submitted an add vote for dposPublicKey.
func HasAddVoted(contract string, dposPublicKey []byte, voterPublicKey []byte) (bool, error) {
	client := spv.GetIPCClient()
	if client == nil {
		return false, errors.New("spv ipc client is nil")
	}
	if !common.IsHexAddress(contract) {
		return false, errors.New("invalid blacklist contract address")
	}
	if len(dposPublicKey) == 0 || len(voterPublicKey) == 0 {
		return false, errors.New("invalid hasVoted parameters")
	}
	inputData, err := blacklistABI.Pack("hasAddVoted", dposPublicKey, voterPublicKey)
	if err != nil {
		return false, err
	}
	contractAddr := common.HexToAddress(contract)
	msg := ethereum.CallMsg{From: common.Address{}, To: &contractAddr, Data: inputData}
	out, err := client.PendingCallContract(context.Background(), msg)
	if err != nil {
		return false, err
	}
	values, err := blacklistABI.Unpack("hasAddVoted", out)
	if err != nil {
		return false, err
	}
	if len(values) != 1 {
		return false, errors.New("invalid hasAddVoted response")
	}
	voted, ok := values[0].(bool)
	if !ok {
		return false, errors.New("invalid hasAddVoted type")
	}
	return voted, nil
}

// HasRemoveVoted checks whether voterPublicKey already submitted a remove vote for dposPublicKey.
func HasRemoveVoted(contract string, dposPublicKey []byte, voterPublicKey []byte) (bool, error) {
	client := spv.GetIPCClient()
	if client == nil {
		return false, errors.New("spv ipc client is nil")
	}
	if !common.IsHexAddress(contract) {
		return false, errors.New("invalid blacklist contract address")
	}
	if len(dposPublicKey) == 0 || len(voterPublicKey) == 0 {
		return false, errors.New("invalid hasRemoveVoted parameters")
	}
	inputData, err := blacklistABI.Pack("hasRemoveVoted", dposPublicKey, voterPublicKey)
	if err != nil {
		return false, err
	}
	contractAddr := common.HexToAddress(contract)
	msg := ethereum.CallMsg{From: common.Address{}, To: &contractAddr, Data: inputData}
	out, err := client.PendingCallContract(context.Background(), msg)
	if err != nil {
		return false, err
	}
	values, err := blacklistABI.Unpack("hasRemoveVoted", out)
	if err != nil {
		return false, err
	}
	if len(values) != 1 {
		return false, errors.New("invalid hasRemoveVoted response")
	}
	voted, ok := values[0].(bool)
	if !ok {
		return false, errors.New("invalid hasRemoveVoted type")
	}
	return voted, nil
}

// GetBlacklistEntry returns the full blacklist entry from the contract.
func GetBlacklistEntry(contract string, dposPublicKey []byte) (*BlacklistEntry, error) {
	client := spv.GetIPCClient()
	if client == nil {
		return nil, errors.New("spv ipc client is nil")
	}
	if !common.IsHexAddress(contract) {
		return nil, errors.New("invalid blacklist contract address")
	}
	if len(dposPublicKey) == 0 {
		return nil, errors.New("dpos public key is empty")
	}
	inputData, err := blacklistABI.Pack("getBlacklistEntry", dposPublicKey)
	if err != nil {
		return nil, err
	}
	contractAddr := common.HexToAddress(contract)
	msg := ethereum.CallMsg{From: common.Address{}, To: &contractAddr, Data: inputData}
	out, err := client.PendingCallContract(context.Background(), msg)
	if err != nil {
		return nil, err
	}
	var result struct {
		Entry BlacklistEntry `abi:"entry"`
	}
	if err := blacklistABI.UnpackIntoInterface(&result, "getBlacklistEntry", out); err != nil {
		return nil, err
	}
	return &result.Entry, nil
}

// IsBlacklistExpired returns whether the blacklist entry is currently expired.
func IsBlacklistExpired(contract string, dposPublicKey []byte) (bool, error) {
	entry, err := GetBlacklistEntry(contract, dposPublicKey)
	if err != nil {
		return false, err
	}
	return BlacklistStatus(entry.Status) == BlacklistStatusExpired, nil
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
	_, err := client.PendingCallContract(context.Background(), msg)
	if err != nil {
		log.Warn("Blacklist vote precheck failed", "error", err)
		return err
	}
	return nil
}

func estimateBlacklistVoteGas(client *ethclient.Client, msg ethereum.CallMsg) (uint64, error) {
	gasLimit, err := client.EstimateGas(context.Background(), msg)
	if err == nil {
		return gasLimit, nil
	}
	return 0, err
}
