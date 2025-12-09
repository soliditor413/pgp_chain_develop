package validators

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync/atomic"

	"github.com/elastos/Elastos.ELA/events"
	ethereum "github.com/pgprotocol/pgp-chain"
	"github.com/pgprotocol/pgp-chain/accounts/abi"
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/consensus/pbft"
	"github.com/pgprotocol/pgp-chain/dpos"
	"github.com/pgprotocol/pgp-chain/log"
	"github.com/pgprotocol/pgp-chain/spv"
)

const validatorABI = `[{"inputs":[],"name":"getValidatorSet","outputs":[{"internalType":"bytes[]","name":"validators","type":"bytes[]"},{"internalType":"uint8","name":"totalValidatorsCount","type":"uint8"}],"stateMutability":"view","type":"function"}]`

type BposValidator struct {
	validatorContract string
	bPosStartHeight   uint64
	initialized       uint32
}

func NewBPosValidator(validatorContract string, bPosStartHeight uint64) *BposValidator {
	return &BposValidator{
		validatorContract: validatorContract,
		bPosStartHeight:   bPosStartHeight,
	}
}

func (v *BposValidator) Start() {
	go v.subscribeSpvEvent()
}

func (v *BposValidator) subscribeSpvEvent() {
	events.Subscribe(func(e *events.Event) {
		switch e.Type {
		case dpos.ETOnSPVHeight:
			height := e.Data.(uint64)
			if spv.PbftEngine == nil {
				return
			}
			pbftEngine, ok := spv.PbftEngine.(*pbft.Pbft)
			if !ok || pbftEngine == nil {
				log.Error("PbftEngine type assertion failed")
				return
			}
			if height > v.bPosStartHeight-5 && height < v.bPosStartHeight {
				curProducers := pbftEngine.GetCurrentProducers()
				isSame := pbftEngine.IsSameProducers(curProducers)
				if !isSame {
					go pbftEngine.AnnounceDAddr()
				} else {
					log.Info("For the same batch of validators, no need to re-connect direct net")
				}

			} else if height >= v.bPosStartHeight && atomic.LoadUint32(&v.initialized) == 0 {
				if err := v.syncValidatorsFromContract(pbftEngine, height); err != nil {
					log.Error("sync validators from contract failed", "height", height, "error", err)
					return
				}
				atomic.StoreUint32(&v.initialized, 1)
				log.Info("BPos validators initialized from contract", "height", height)
			}
		}
	})
}

func (v *BposValidator) syncValidatorsFromContract(pbftEngine *pbft.Pbft, height uint64) error {
	validators, totalCount, err := v.getCurrentValidators(height)
	if err != nil {
		return err
	}
	if len(validators) == 0 {
		return errors.New("validator set is empty")
	}
	pbftEngine.UpdateCurrentProducers(validators, int(totalCount), height)
	go pbftEngine.AnnounceDAddr()
	return nil
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

func (v *BposValidator) Stop() error {
	return nil
}
