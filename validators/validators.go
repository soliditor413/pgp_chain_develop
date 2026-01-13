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

	ethereum "github.com/pgprotocol/pgp-chain"
	"github.com/pgprotocol/pgp-chain/accounts/abi"
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/core/types"
	"github.com/pgprotocol/pgp-chain/log"
	"github.com/pgprotocol/pgp-chain/spv"
)

const validatorABI = `[{"inputs":[],"name":"getNextValidatorSet","outputs":[{"internalType":"bytes[]","name":"validators","type":"bytes[]"},{"internalType":"uint8","name":"totalValidatorsCount","type":"uint8"}],"stateMutability":"view","type":"function"},{"inputs":[],"name":"getEpoch0Validators","outputs":[{"internalType":"bytes[]","name":"validators","type":"bytes[]"},{"internalType":"uint8","name":"totalValidatorsCount","type":"uint8"}],"stateMutability":"view","type":"function"}]`

type BposValidator struct {
	validatorContract  string
	bPosStartHeight    uint64
	nextTurnValidators *NextTurnValidators
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

func (v *BposValidator) OnBlockEvent(block *types.Block) bool {
	fmt.Println("BposValidator OnBlockEvent", block.NumberU64(), " bPosStartHeight ", v.bPosStartHeight)
	if v.validatorContract == "" {
		return false
	}
	if block.NumberU64() < v.bPosStartHeight {
		return false
	}
	offset := block.NumberU64() - v.bPosStartHeight
	if offset%36 != 0 {
		return false
	}
	validators, totalCount, err := v.GetCurrentValidatorSet(block.NumberU64())
	if err != nil {
		log.Error("OnBlockEvent", "getCurrentValidators error", err)
		return false
	}
	workingHeight := block.NumberU64() + 36
	if v.IsSameLastNextTurnValidators(validators) {
		return false
	}
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

func (v *BposValidator) GetCurrentValidatorSet(height uint64) ([][]byte, uint8, error) {
	if v.validatorContract == "" {
		return nil, 0, errors.New("validator contract address is empty")
	}
	if !common.IsHexAddress(v.validatorContract) {
		return nil, 0, errors.New("validator contract address is invalid")
	}
	epoch := (height - v.bPosStartHeight) / 36
	fmt.Println(">>>>>>>>>>> GetCurrentValidatorSet <<<<<<<<<<< epoch ", epoch)
	if epoch == 0 {
		return v.GetEpoch0Validators(height)
	}
	return v.GetNextValidatorSet(height - 36)
}

func (v *BposValidator) GetEpoch0Validators(height uint64) ([][]byte, uint8, error) {
	if v.validatorContract == "" {
		return nil, 0, errors.New("validator contract address is empty")
	}
	if !common.IsHexAddress(v.validatorContract) {
		return nil, 0, errors.New("validator contract address is invalid")
	}
	client := spv.GetClient()
	if client == nil {
		return nil, 0, errors.New("spv eth client is nil")
	}
	contractABI, err := abi.JSON(strings.NewReader(validatorABI))
	if err != nil {
		return nil, 0, err
	}
	contractAddr := common.HexToAddress(v.validatorContract)
	data, err := contractABI.Pack("getEpoch0Validators")
	if err != nil {
		return nil, 0, err
	}
	msg := ethereum.CallMsg{
		To:   &contractAddr,
		Data: data,
	}
	blockNum := new(big.Int).SetUint64(height)
	output, err := client.CallContract(context.Background(), msg, blockNum)
	if err != nil {
		return nil, 0, err
	}
	if len(output) == 0 {
		return nil, 0, errors.New("empty response from getEpoch0Validators")
	}
	var resp struct {
		Validators           [][]byte
		TotalValidatorsCount uint8
	}
	if err := contractABI.UnpackIntoInterface(&resp, "getEpoch0Validators", output); err != nil {
		return nil, 0, err
	}
	return resp.Validators, resp.TotalValidatorsCount, nil
}

func (v *BposValidator) GetNextValidatorSet(height uint64) ([][]byte, uint8, error) {
	if v.validatorContract == "" {
		return nil, 0, errors.New("validator contract address is empty")
	}
	if !common.IsHexAddress(v.validatorContract) {
		return nil, 0, errors.New("validator contract address is invalid")
	}
	client := spv.GetClient()
	if client == nil {
		return nil, 0, errors.New("spv eth client is nil")
	}
	contractABI, err := abi.JSON(strings.NewReader(validatorABI))
	if err != nil {
		return nil, 0, err
	}
	contractAddr := common.HexToAddress(v.validatorContract)
	data, err := contractABI.Pack("getNextValidatorSet")
	if err != nil {
		return nil, 0, err
	}
	msg := ethereum.CallMsg{
		To:   &contractAddr,
		Data: data,
	}
	blockNum := new(big.Int).SetUint64(height)
	output, err := client.CallContract(context.Background(), msg, blockNum)
	if err != nil {
		return nil, 0, err
	}
	if len(output) == 0 {
		return nil, 0, errors.New("empty response from getNextValidatorSet")
	}
	var resp struct {
		Validators           [][]byte
		TotalValidatorsCount uint8
	}
	if err := contractABI.UnpackIntoInterface(&resp, "getNextValidatorSet", output); err != nil {
		return nil, 0, err
	}
	return resp.Validators, resp.TotalValidatorsCount, nil
}
