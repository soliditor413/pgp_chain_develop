package validators

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/elastos/Elastos.ELA/events"
	"github.com/pgprotocol/pgp-chain/dpos"

	"github.com/pgprotocol/pgp-chain/accounts/abi"
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/core/types"
	"github.com/pgprotocol/pgp-chain/log"
	"github.com/pgprotocol/pgp-chain/rpc"
)

const validatorABI = `[{"inputs":[],"name":"getNextValidatorSet","outputs":[{"internalType":"bytes[]","name":"validators","type":"bytes[]"},{"internalType":"uint8","name":"totalValidatorsCount","type":"uint8"},{"internalType":"uint256","name":"workingHeight","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"uint256","name":"epoch","type":"uint256"}],"name":"getCachedValidatorSet","outputs":[{"internalType":"bytes[]","name":"validators","type":"bytes[]"},{"internalType":"uint8","name":"totalValidatorsCount","type":"uint8"},{"internalType":"uint256","name":"workingHeight","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"uint256","name":"epoch","type":"uint256"}],"name":"isValidatorSetCached","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"view","type":"function"}]`
const BLOCKS_PER_EPOCH = 18 //TODO test for jianbin,should change to 36

// ContractCaller abstracts contract call capability so that BposValidator
// can call the validator contract in-process (via ethapi) without depending
// on the ethclient IPC path. The signature mirrors ethapi.PublicBlockChainAPI.Call.
type ContractCaller interface {
	Call(ctx context.Context, to common.Address, data []byte, blockNrOrHash rpc.BlockNumberOrHash) ([]byte, error)
}

type BposValidator struct {
	validatorContract  string
	bPosStartHeight    uint64
	nextTurnValidators *NextTurnValidators
	caller             ContractCaller
}

type validatorSetResult struct {
	Validators           [][]byte
	TotalValidatorsCount uint8
	WorkingHeight        *big.Int
}

func NewBPosValidator(
	validatorContract string,
	bPosStartHeight uint64,
) (*BposValidator, error) {
	return &BposValidator{
		validatorContract: validatorContract,
		bPosStartHeight:   bPosStartHeight,
	}, nil
}

// SetContractCaller injects the in-process contract caller after the blockchain is ready.
func (v *BposValidator) SetContractCaller(caller ContractCaller) {
	v.caller = caller
}

func (v *BposValidator) ValidatorContract() string {
	return v.validatorContract
}

func (v *BposValidator) BPosStartHeight() uint64 {
	return v.bPosStartHeight
}

func (v *BposValidator) OnBlockEvent(block *types.Block) bool {
	fmt.Println("BposValidator OnBlockEvent", block.NumberU64(), " bPosStartHeight ", v.bPosStartHeight)
	if v.validatorContract == "" {
		return false
	}
	if block.NumberU64() < v.bPosStartHeight {
		return false
	}
	if block.NumberU64() > v.bPosStartHeight {
		offset := block.NumberU64() - v.bPosStartHeight
		if offset%BLOCKS_PER_EPOCH != 0 {
			return false
		}
	}
	epoch := uint64(0)
	if block.NumberU64() >= v.bPosStartHeight {
		epoch = (block.NumberU64() - v.bPosStartHeight) / BLOCKS_PER_EPOCH
	}
	var (
		validators    [][]byte
		totalCount    uint8
		workingHeight uint64
		err           error
	)
	// Try on-chain cache first (at latest state), then fall back to block hash state.
	validators, totalCount, workingHeight, err = v.getCachedValidatorSetWithEpoch(epoch)
	fmt.Println(">>>>> 11 getCachedValidatorSetWithEpoch working height", workingHeight)
	if err != nil || len(validators) == 0 {
		validators, totalCount, workingHeight, err = v.getNextValidatorSetWithHeight(block.Hash())
		fmt.Println(">>>>> 22 getNextValidatorSetWithHeight working height", workingHeight)
	}
	if err != nil {
		log.Error("OnBlockEvent", "getCurrentValidators error", err)
		return false
	}
	if v.IsSameLastNextTurnValidators(validators) {
		fmt.Println("is same nextTurn validators")
		return false
	}
	fmt.Println(">>>>> current working height", workingHeight, " block height ", block.NumberU64())
	v.nextTurnValidators = NewNextTurnValidators(workingHeight, validators, int(totalCount))
	v.dumpValidators()
	events.Notify(dpos.ETNextValidators, *v.nextTurnValidators)
	return true
}

func (v *BposValidator) IsSameLastNextTurnValidators(validators [][]byte) bool {
	if v.nextTurnValidators == nil {
		return false
	}
	if len(validators) != len(v.nextTurnValidators.Validators) {
		return false
	}
	for index, p := range validators {
		if !bytes.Equal(p, v.nextTurnValidators.Validators[index][:]) {
			return false
		}
	}
	return true
}

func (v *BposValidator) IsWorkingHeight(height uint64) bool {
	if v.nextTurnValidators == nil {
		return false
	}
	fmt.Println("IsWorkingHeight", "height ", height, "v.nextTurnValidators.WorkingHeight", v.nextTurnValidators.WorkingHeight)
	return height >= v.nextTurnValidators.WorkingHeight
}

func (v *BposValidator) WorkingHeight() uint64 {
	if v.nextTurnValidators == nil {
		return 0
	}
	return v.nextTurnValidators.WorkingHeight
}

func (v *BposValidator) IsBPosFork(height uint64) bool {
	return height >= v.bPosStartHeight
}

func (v *BposValidator) dumpValidators() {
	log.Info("-------------------dump next turn validators---------------")
	fmt.Println("workingHeight ", v.nextTurnValidators.WorkingHeight, " totalCount ", v.nextTurnValidators.TotalCount)
	fmt.Println("----------------------------------------")
	for _, v := range v.nextTurnValidators.Validators {
		fmt.Println(common.Bytes2Hex(v))
	}
	fmt.Println("----------------------------------------")
}

// GetCurrentValidatorSet queries the validator contract for the current validator set.
func (v *BposValidator) GetCurrentValidatorSet(height uint64) ([][]byte, uint8, uint64, error) {
	if v.validatorContract == "" {
		return nil, 0, 0, errors.New("validator contract address is empty")
	}
	if !common.IsHexAddress(v.validatorContract) {
		return nil, 0, 0, errors.New("validator contract address is invalid")
	}
	var epoch uint64 = 0
	if height > v.bPosStartHeight {
		epoch = (height - v.bPosStartHeight) / BLOCKS_PER_EPOCH
	}
	log.Info(">>>>>>>>>>> GetCurrentValidatorSet <<<<<<<<<<< ", "epoch:", epoch)
	if epoch != 0 && (height-v.bPosStartHeight)%BLOCKS_PER_EPOCH == 0 {
		epoch = epoch - 1
	}

	validators, count, workingHeight, err := v.getCachedValidatorSetWithEpoch(epoch)
	if err == nil && len(validators) > 0 {
		return validators, count, workingHeight, nil
	}
	return v.getValidatorsByNumberWithHeight("getNextValidatorSet", height)
}

// GetNextValidatorSet queries the contract at the given block hash.
func (v *BposValidator) GetNextValidatorSet(blockHash common.Hash) ([][]byte, uint8, uint64, error) {
	validators, totalCount, workingHeight, err := v.getNextValidatorSetWithHeight(blockHash)
	return validators, totalCount, workingHeight, err
}

// GetNextValidatorSetByNumber queries the contract at the given block number (used when block hash is unavailable).
func (v *BposValidator) GetNextValidatorSetByNumber(height uint64) ([][]byte, uint8, error) {
	validators, totalCount, _, err := v.getValidatorsByNumberWithHeight("getNextValidatorSet", height)
	return validators, totalCount, err
}

func (v *BposValidator) getNextValidatorSetWithHeight(blockHash common.Hash) ([][]byte, uint8, uint64, error) {
	return v.callValidatorContractWithHeight("getNextValidatorSet", blockHash)
}

// callValidatorContractWithHeight calls the validator contract method at a specific
// block hash and also decodes the workingHeight returned by the contract.
func (v *BposValidator) callValidatorContractWithHeight(method string, blockHash common.Hash) ([][]byte, uint8, uint64, error) {
	if v.validatorContract == "" {
		return nil, 0, 0, errors.New("validator contract address is empty")
	}
	if !common.IsHexAddress(v.validatorContract) {
		return nil, 0, 0, errors.New("validator contract address is invalid")
	}
	if v.caller == nil {
		return nil, 0, 0, errors.New("contract caller is not configured (blockchain not ready)")
	}
	contractABI, err := abi.JSON(strings.NewReader(validatorABI))
	if err != nil {
		return nil, 0, 0, err
	}
	data, err := contractABI.Pack(method)
	if err != nil {
		return nil, 0, 0, err
	}
	contractAddr := common.HexToAddress(v.validatorContract)
	blockNrOrHash := rpc.BlockNumberOrHashWithHash(blockHash, false)
	output, err := v.caller.Call(context.Background(), contractAddr, data, blockNrOrHash)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("%s call failed: %w", method, err)
	}
	if len(output) == 0 {
		return nil, 0, 0, fmt.Errorf("empty response from %s", method)
	}
	var resp validatorSetResult
	if err := contractABI.UnpackIntoInterface(&resp, method, output); err != nil {
		return nil, 0, 0, err
	}
	return resp.Validators, resp.TotalValidatorsCount, uint64Value(resp.WorkingHeight), nil
}

// getValidatorsByNumber calls the validator contract at a specific block number.
func (v *BposValidator) getValidatorsByNumber(method string, height uint64) ([][]byte, uint8, error) {
	validators, totalCount, _, err := v.getValidatorsByNumberWithHeight(method, height)
	return validators, totalCount, err
}

func (v *BposValidator) getValidatorsByNumberWithHeight(method string, height uint64) ([][]byte, uint8, uint64, error) {
	if v.validatorContract == "" {
		return nil, 0, 0, errors.New("validator contract address is empty")
	}
	if !common.IsHexAddress(v.validatorContract) {
		return nil, 0, 0, errors.New("validator contract address is invalid")
	}
	if v.caller == nil {
		return nil, 0, 0, errors.New("contract caller is not configured (blockchain not ready)")
	}
	contractABI, err := abi.JSON(strings.NewReader(validatorABI))
	if err != nil {
		return nil, 0, 0, err
	}
	data, err := contractABI.Pack(method)
	if err != nil {
		return nil, 0, 0, err
	}
	contractAddr := common.HexToAddress(v.validatorContract)
	// blockNr := rpc.BlockNumber(height)
	blockNrOrHash := rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber) //// rpc.BlockNumberOrHashWithNumber(blockNr)
	output, err := v.caller.Call(context.Background(), contractAddr, data, blockNrOrHash)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("%s call failed at height %d: %w", method, height, err)
	}
	if len(output) == 0 {
		return nil, 0, 0, fmt.Errorf("empty response from %s at height %d", method, height)
	}
	var resp validatorSetResult
	if err := contractABI.UnpackIntoInterface(&resp, method, output); err != nil {
		return nil, 0, 0, err
	}
	return resp.Validators, resp.TotalValidatorsCount, uint64Value(resp.WorkingHeight), nil
}

// GetCachedValidatorSet queries the contract's getCachedValidatorSet(epoch) at the latest block.
// Returns the cached validator set if available, or an error if not cached / call fails.
func (v *BposValidator) GetCachedValidatorSet(epoch uint64) ([][]byte, uint8, uint64, error) {
	validators, totalCount, workingHeight, err := v.getCachedValidatorSetWithEpoch(epoch)
	return validators, totalCount, workingHeight, err
}

func (v *BposValidator) getCachedValidatorSetWithEpoch(epoch uint64) ([][]byte, uint8, uint64, error) {
	if v.caller == nil {
		return nil, 0, 0, errors.New("contract caller is not configured")
	}
	contractABI, err := abi.JSON(strings.NewReader(validatorABI))
	if err != nil {
		return nil, 0, 0, err
	}
	data, err := contractABI.Pack("getCachedValidatorSet", new(big.Int).SetUint64(epoch))
	if err != nil {
		return nil, 0, 0, err
	}
	contractAddr := common.HexToAddress(v.validatorContract)
	blockNrOrHash := rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
	output, err := v.caller.Call(context.Background(), contractAddr, data, blockNrOrHash)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("getCachedValidatorSet call failed for epoch %d: %w", epoch, err)
	}
	if len(output) == 0 {
		return nil, 0, 0, fmt.Errorf("empty response from getCachedValidatorSet for epoch %d", epoch)
	}
	var resp validatorSetResult
	if err := contractABI.UnpackIntoInterface(&resp, "getCachedValidatorSet", output); err != nil {
		return nil, 0, 0, err
	}
	if len(resp.Validators) == 0 {
		return nil, 0, 0, fmt.Errorf("no cached validators for epoch %d", epoch)
	}
	return resp.Validators, resp.TotalValidatorsCount, uint64Value(resp.WorkingHeight), nil
}

// isValidatorSetCached queries isValidatorSetCached(epoch) on the contract at the latest block.
func (v *BposValidator) isValidatorSetCached(epoch uint64) bool {
	if v.caller == nil {
		return false
	}
	contractABI, err := abi.JSON(strings.NewReader(validatorABI))
	if err != nil {
		return false
	}
	data, err := contractABI.Pack("isValidatorSetCached", new(big.Int).SetUint64(epoch))
	if err != nil {
		return false
	}
	contractAddr := common.HexToAddress(v.validatorContract)
	blockNrOrHash := rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
	output, err := v.caller.Call(context.Background(), contractAddr, data, blockNrOrHash)
	if err != nil {
		return false
	}
	var result struct {
		Cached bool
	}
	if err := contractABI.UnpackIntoInterface(&result, "isValidatorSetCached", output); err != nil {
		return false
	}
	return result.Cached
}

// EthAPICaller implements ContractCaller using ethapi.PublicBlockChainAPI.
// It lives here to keep the interface and default impl together; wired in eth/backend.go.
type EthAPICaller struct {
	callFn func(ctx context.Context, to common.Address, data []byte, blockNrOrHash rpc.BlockNumberOrHash) ([]byte, error)
}

func NewEthAPICaller(callFn func(ctx context.Context, to common.Address, data []byte, blockNrOrHash rpc.BlockNumberOrHash) ([]byte, error)) *EthAPICaller {
	return &EthAPICaller{callFn: callFn}
}

func (c *EthAPICaller) Call(ctx context.Context, to common.Address, data []byte, blockNrOrHash rpc.BlockNumberOrHash) ([]byte, error) {
	return c.callFn(ctx, to, data, blockNrOrHash)
}

func uint64Value(value *big.Int) uint64 {
	if value == nil || value.Sign() <= 0 {
		return 0
	}
	return value.Uint64()
}
