package validators

import (
	"github.com/elastos/Elastos.ELA/events"
	"github.com/pgprotocol/pgp-chain/consensus/pbft"
	"github.com/pgprotocol/pgp-chain/dpos"
	"github.com/pgprotocol/pgp-chain/log"
	"github.com/pgprotocol/pgp-chain/spv"
)

type BposValidator struct {
	validatorContract string
	bPosStartHeight   uint64
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
			pbftEngine := spv.PbftEngine.(*pbft.Pbft)
			if spv.PbftEngine == nil {
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

			} else if height == v.bPosStartHeight {

			}
		}
	})
}

func (v *BposValidator) getCurrentValidators() [][]byte {
	validators := make([][]byte, 0)
	return validators
}

func (v *BposValidator) Stop() error {
	return nil
}
