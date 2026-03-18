package validators

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/elastos/Elastos.ELA/events"
	"github.com/pgprotocol/pgp-chain/dpos"

	"github.com/pgprotocol/pgp-chain/accounts/abi"
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/core/types"
	"github.com/pgprotocol/pgp-chain/log"
	"github.com/pgprotocol/pgp-chain/rpc"
)

const validatorABI = `[{"inputs":[],"name":"getNextValidatorSet","outputs":[{"internalType":"bytes[]","name":"validators","type":"bytes[]"},{"internalType":"uint8","name":"totalValidatorsCount","type":"uint8"}],"stateMutability":"view","type":"function"},{"inputs":[],"name":"getEpoch0Validators","outputs":[{"internalType":"bytes[]","name":"validators","type":"bytes[]"},{"internalType":"uint8","name":"totalValidatorsCount","type":"uint8"}],"stateMutability":"view","type":"function"}]`
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

func (v *BposValidator) OnBlockEvent(block *types.Block) bool {
	fmt.Println("BposValidator OnBlockEvent", block.NumberU64(), " bPosStartHeight ", v.bPosStartHeight)
	if v.validatorContract == "" {
		return false
	}
	if block.NumberU64() < v.bPosStartHeight-BLOCKS_PER_EPOCH {
		return false
	}
	if block.NumberU64() > v.bPosStartHeight {
		offset := block.NumberU64() - v.bPosStartHeight
		if offset%BLOCKS_PER_EPOCH != 0 {
			return false
		}
	}
	validators := make([][]byte, 0)
	totalCount := uint8(0)
	var err error
	if block.NumberU64() < v.bPosStartHeight && block.NumberU64() > v.bPosStartHeight-BLOCKS_PER_EPOCH {
		validators, totalCount, err = v.GetEpoch0Validators(block.Hash())
	} else {
		validators, totalCount, err = v.GetNextValidatorSet(block.Hash())
	}

	if err != nil {
		log.Error("OnBlockEvent", "getCurrentValidators error", err)
		return false
	}
	if v.IsSameLastNextTurnValidators(validators) {
		fmt.Println("is same nextTurn validators")
		return false
	}
	workingHeight := block.NumberU64() + BLOCKS_PER_EPOCH
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
// blockHash is the hash of the block whose state should be used.
func (v *BposValidator) GetCurrentValidatorSet(blockHash common.Hash, height uint64) ([][]byte, uint8, error) {
	if v.validatorContract == "" {
		return nil, 0, errors.New("validator contract address is empty")
	}
	if !common.IsHexAddress(v.validatorContract) {
		return nil, 0, errors.New("validator contract address is invalid")
	}
	if height < v.bPosStartHeight {
		return nil, 0, errors.New("height is less than bPosStartHeight")
	}
	epoch := (height - v.bPosStartHeight) / BLOCKS_PER_EPOCH
	fmt.Println(">>>>>>>>>>> GetCurrentValidatorSet <<<<<<<<<<< epoch ", epoch)
	if epoch == 0 {
		return v.GetEpoch0Validators(blockHash)
	}
	// For non-zero epoch we need the state at (height - BLOCKS_PER_EPOCH).
	// We don't have that block's hash here, so fall back to block number query.
	return v.getValidatorsByNumber("getNextValidatorSet", height-BLOCKS_PER_EPOCH)
}

// GetEpoch0Validators queries the contract at the given block hash.
func (v *BposValidator) GetEpoch0Validators(blockHash common.Hash) ([][]byte, uint8, error) {
	return v.callValidatorContract("getEpoch0Validators", blockHash)
}

// GetNextValidatorSet queries the contract at the given block hash.
func (v *BposValidator) GetNextValidatorSet(blockHash common.Hash) ([][]byte, uint8, error) {
	return v.callValidatorContract("getNextValidatorSet", blockHash)
}

// GetNextValidatorSetByNumber queries the contract at the given block number (used when block hash is unavailable).
func (v *BposValidator) GetNextValidatorSetByNumber(height uint64) ([][]byte, uint8, error) {
	return v.getValidatorsByNumber("getNextValidatorSet", height)
}

// callValidatorContract calls the validator contract method at a specific block hash (in-process, state guaranteed in memory).
func (v *BposValidator) callValidatorContract(method string, blockHash common.Hash) ([][]byte, uint8, error) {
	if v.validatorContract == "" {
		return nil, 0, errors.New("validator contract address is empty")
	}
	if !common.IsHexAddress(v.validatorContract) {
		return nil, 0, errors.New("validator contract address is invalid")
	}
	if v.caller == nil {
		return nil, 0, errors.New("contract caller is not configured (blockchain not ready)")
	}
	contractABI, err := abi.JSON(strings.NewReader(validatorABI))
	if err != nil {
		return nil, 0, err
	}
	data, err := contractABI.Pack(method)
	if err != nil {
		return nil, 0, err
	}
	contractAddr := common.HexToAddress(v.validatorContract)
	blockNrOrHash := rpc.BlockNumberOrHashWithHash(blockHash, false)
	output, err := v.caller.Call(context.Background(), contractAddr, data, blockNrOrHash)
	if err != nil {
		return nil, 0, fmt.Errorf("%s call failed: %w", method, err)
	}
	if len(output) == 0 {
		return nil, 0, fmt.Errorf("empty response from %s", method)
	}
	var resp struct {
		Validators           [][]byte
		TotalValidatorsCount uint8
	}
	if err := contractABI.UnpackIntoInterface(&resp, method, output); err != nil {
		return nil, 0, err
	}
	return resp.Validators, resp.TotalValidatorsCount, nil
}

// getValidatorsByNumber calls the validator contract at a specific block number.
func (v *BposValidator) getValidatorsByNumber(method string, height uint64) ([][]byte, uint8, error) {
	if v.validatorContract == "" {
		return nil, 0, errors.New("validator contract address is empty")
	}
	if !common.IsHexAddress(v.validatorContract) {
		return nil, 0, errors.New("validator contract address is invalid")
	}
	if v.caller == nil {
		return nil, 0, errors.New("contract caller is not configured (blockchain not ready)")
	}
	contractABI, err := abi.JSON(strings.NewReader(validatorABI))
	if err != nil {
		return nil, 0, err
	}
	data, err := contractABI.Pack(method)
	if err != nil {
		return nil, 0, err
	}
	contractAddr := common.HexToAddress(v.validatorContract)
	blockNr := rpc.BlockNumber(height)
	blockNrOrHash := rpc.BlockNumberOrHashWithNumber(blockNr)
	output, err := v.caller.Call(context.Background(), contractAddr, data, blockNrOrHash)
	if err != nil {
		return nil, 0, fmt.Errorf("%s call failed at height %d: %w", method, height, err)
	}
	if len(output) == 0 {
		return nil, 0, fmt.Errorf("empty response from %s at height %d", method, height)
	}
	var resp struct {
		Validators           [][]byte
		TotalValidatorsCount uint8
	}
	if err := contractABI.UnpackIntoInterface(&resp, method, output); err != nil {
		return nil, 0, err
	}
	return resp.Validators, resp.TotalValidatorsCount, nil
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
