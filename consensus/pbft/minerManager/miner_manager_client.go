package minermanager

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

const minerManagerABI = `[
	{
		"inputs": [{"internalType":"bytes","name":"producerPublicKey","type":"bytes"},{"internalType":"bytes","name":"signature","type":"bytes"}],
		"name": "cacheValidatorSet",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [{"internalType":"uint256","name":"epoch","type":"uint256"}],
		"name": "isValidatorSetCached",
		"outputs": [{"internalType":"bool","name":"","type":"bool"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [{"internalType":"bytes","name":"producerPublicKey","type":"bytes"}],
		"name": "getCacheNonce",
		"outputs": [{"internalType":"uint256","name":"","type":"uint256"}],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "getCurrentEpoch",
		"outputs": [{"internalType":"uint256","name":"","type":"uint256"}],
		"stateMutability": "view",
		"type": "function"
	}
]`

var parsedABI abi.ABI

func init() {
	parsed, err := abi.JSON(strings.NewReader(minerManagerABI))
	if err != nil {
		panic(err)
	}
	parsedABI = parsed
}

func getClient() (*ethclient.Client, error) {
	client := spv.GetIPCClient()
	if client == nil {
		return nil, errors.New("spv ipc client is nil")
	}
	return client, nil
}

func IsValidatorSetCached(contract string, epoch *big.Int) (bool, error) {
	client, err := getClient()
	if err != nil {
		return false, err
	}
	contractAddr := common.HexToAddress(contract)
	data, err := parsedABI.Pack("isValidatorSetCached", epoch)
	if err != nil {
		return false, err
	}
	result, err := client.CallContract(context.Background(), ethereum.CallMsg{
		To:   &contractAddr,
		Data: data,
	}, nil)
	if err != nil {
		return false, err
	}
	var cached bool
	if err := parsedABI.UnpackIntoInterface(&cached, "isValidatorSetCached", result); err != nil {
		return false, err
	}
	return cached, nil
}

func GetCurrentEpoch(contract string) (*big.Int, error) {
	client, err := getClient()
	if err != nil {
		return nil, err
	}
	contractAddr := common.HexToAddress(contract)
	data, err := parsedABI.Pack("getCurrentEpoch")
	if err != nil {
		return nil, err
	}
	result, err := client.CallContract(context.Background(), ethereum.CallMsg{
		To:   &contractAddr,
		Data: data,
	}, nil)
	if err != nil {
		return nil, err
	}
	var epoch *big.Int
	if err := parsedABI.UnpackIntoInterface(&epoch, "getCurrentEpoch", result); err != nil {
		return nil, err
	}
	return epoch, nil
}

func GetCacheNonce(contract string, producerPublicKey []byte) (*big.Int, error) {
	client, err := getClient()
	if err != nil {
		return nil, err
	}
	contractAddr := common.HexToAddress(contract)
	data, err := parsedABI.Pack("getCacheNonce", producerPublicKey)
	if err != nil {
		return nil, err
	}
	result, err := client.CallContract(context.Background(), ethereum.CallMsg{
		To:   &contractAddr,
		Data: data,
	}, nil)
	if err != nil {
		return nil, err
	}
	var nonce *big.Int
	if err := parsedABI.UnpackIntoInterface(&nonce, "getCacheNonce", result); err != nil {
		return nil, err
	}
	return nonce, nil
}

func SendCacheValidatorSet(contract string, producerPublicKey []byte, signature []byte) (common.Hash, error) {
	client, err := getClient()
	if err != nil {
		return common.Hash{}, err
	}
	if !common.IsHexAddress(contract) {
		return common.Hash{}, errors.New("invalid contract address")
	}
	if spv.GetDefaultSingerAddr == nil {
		return common.Hash{}, errors.New("default signer address is not configured")
	}
	from := spv.GetDefaultSingerAddr()
	if (from == common.Address{}) {
		return common.Hash{}, errors.New("default signer address is empty")
	}
	inputData, err := parsedABI.Pack("cacheValidatorSet", producerPublicKey, signature)
	if err != nil {
		return common.Hash{}, err
	}
	contractAddr := common.HexToAddress(contract)
	gasLimit := uint64(3000000)
	gasprice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return common.Hash{}, err
	}
	price := new(big.Int).Mul(gasprice, big.NewInt(110))
	price.Div(price, big.NewInt(100))
	nonce, err := client.PendingNonceAt(context.Background(), from)
	if err != nil {
		return common.Hash{}, err
	}
	callMsg := ethereum.TXMsg{
		From:     from,
		To:       &contractAddr,
		Gas:      gasLimit,
		Data:     inputData,
		GasPrice: price,
		Nonce:    nonce,
	}
	txHash, err := client.SendPublicTransaction(context.Background(), callMsg)
	if err != nil {
		return common.Hash{}, err
	}
	log.Info("Sent cacheValidatorSet tx", "txHash", txHash.String())
	return txHash, nil
}

// CacheValidatorSetMethodID returns the 4-byte selector for cacheValidatorSet(bytes,bytes).
func CacheValidatorSetMethodID() []byte {
	return parsedABI.Methods["cacheValidatorSet"].ID
}

// BuildCacheMessage builds the message that the contract expects for signature verification:
// abi.encodePacked(contractAddress, chainId, epoch, nonce)
func BuildCacheMessage(contractAddr common.Address, chainID *big.Int, epoch *big.Int, nonce *big.Int) []byte {
	var buf []byte
	buf = append(buf, contractAddr.Bytes()...)
	buf = append(buf, encodeUint256(chainID)...)
	buf = append(buf, encodeUint256(epoch)...)
	buf = append(buf, encodeUint256(nonce)...)
	return buf
}

func encodeUint256(value *big.Int) []byte {
	if value == nil {
		return make([]byte, 32)
	}
	b := value.Bytes()
	padded := make([]byte, 32)
	if len(b) > 32 {
		copy(padded, b[len(b)-32:])
	} else {
		copy(padded[32-len(b):], b)
	}
	return padded
}
