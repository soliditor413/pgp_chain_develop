package validators

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/pgprotocol/pgp-chain/core/types"
	"github.com/pgprotocol/pgp-chain/log"

	ethereum "github.com/pgprotocol/pgp-chain"
	"github.com/pgprotocol/pgp-chain/accounts/abi"
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/consensus/pbft"
	"github.com/pgprotocol/pgp-chain/spv"
)

const validatorABI = `[{"inputs":[],"name":"getValidatorSet","outputs":[{"internalType":"bytes[]","name":"validators","type":"bytes[]"},{"internalType":"uint8","name":"totalValidatorsCount","type":"uint8"}],"stateMutability":"view","type":"function"}]`

type BposValidator struct {
	validatorContract string
	bPosStartHeight   uint64
}

func NewBPosValidator(
	validatorContract string,
	bPosStartHeight uint64,
) (*BposValidator, error) {
	if len(validatorContract) == 0 {
		return nil, errors.New("empty validator contract")
	}
	return &BposValidator{
		validatorContract: validatorContract,
		bPosStartHeight:   bPosStartHeight,
	}, nil
}

func (v *BposValidator) OnBlockEvent(block *types.Block) {
	fmt.Println("BposValidator OnBlockEvent", block.NumberU64())
	if block.NumberU64() < v.bPosStartHeight {
		return
	}
	validators, totalCount, err := v.getCurrentValidators(block.NumberU64())
	if err != nil {
		log.Error("OnBlockEvent", "getCurrentValidators error", err)
		return
	}
	v.dumpValidators(block.NumberU64(), validators)
	pbftEngine, ok := spv.PbftEngine.(*pbft.Pbft)
	if !ok || pbftEngine == nil {
		log.Error("PbftEngine type assertion failed")
		return
	}
	if pbftEngine.IsCurrentProducers(validators) {
		return
	}
	pbftEngine.UpdateCurrentProducers(validators, int(totalCount), 0)
	go pbftEngine.AnnounceDAddr()
}

// func (v *BposValidator) subscribeSpvEvent() {

// 	events.Subscribe(func(e *events.Event) {
// 		switch e.Type {
// 		case dpos.ETOnSPVHeight:
// 			height := e.Data.(uint64)
// 			if spv.PbftEngine == nil {
// 				return
// 			}
// 			pbftEngine, ok := spv.PbftEngine.(*pbft.Pbft)
// 			if !ok || pbftEngine == nil {
// 				log.Error("PbftEngine type assertion failed")
// 				return
// 			}
// 			if height > v.bPosStartHeight-5 && height < v.bPosStartHeight {
// 				curProducers := pbftEngine.GetCurrentProducers()
// 				isSame := pbftEngine.IsSameProducers(curProducers)
// 				if !isSame {
// 					go pbftEngine.AnnounceDAddr()
// 				} else {
// 					log.Info("For the same batch of validators, no need to re-connect direct net")
// 				}

// 			} else if height >= v.bPosStartHeight && atomic.LoadUint32(&v.initialized) == 0 {
// 				number := pbftEngine.GetBlockChain().CurrentBlock().GetHeight()
// 				if err := v.syncValidatorsFromContract(pbftEngine, number); err != nil {
// 					log.Error("sync validators from contract failed", "height", height, "error", err)
// 					return
// 				}
// 				atomic.StoreUint32(&v.initialized, 1)
// 				log.Info("BPos validators initialized from contract", "height", height)
// 			}
// 		}
// 	})
// }

func (v *BposValidator) syncValidatorsFromContract(pbftEngine *pbft.Pbft, height uint64) error {
	validators, totalCount, err := v.getCurrentValidators(height)
	if err != nil {
		return err
	}
	if len(validators) == 0 {
		return errors.New("validator set is empty")
	}
	v.dumpValidators(height, validators)
	if pbftEngine.IsCurrentProducers(validators) {
		return nil
	}
	pbftEngine.UpdateCurrentProducers(validators, int(totalCount), 0)
	go pbftEngine.AnnounceDAddr()
	return nil
}

func (v *BposValidator) dumpValidators(height uint64, validators [][]byte) {
	fmt.Println("----------------------------------------")
	fmt.Println("height", height)
	fmt.Println("----------------------------------------")
	for _, v := range validators {
		fmt.Println(common.Bytes2Hex(v))
	}
	fmt.Println("----------------------------------------")
}

func (v *BposValidator) getCurrentValidators(height uint64) ([][]byte, uint8, error) {
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
	data, err := contractABI.Pack("getValidatorSet")
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
		return nil, 0, errors.New("empty response from getValidatorSet")
	}
	var resp struct {
		Validators           [][]byte
		TotalValidatorsCount uint8
	}
	if err := contractABI.UnpackIntoInterface(&resp, "getValidatorSet", output); err != nil {
		return nil, 0, err
	}
	return resp.Validators, resp.TotalValidatorsCount, nil
}
