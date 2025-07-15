package spv

import (
	"bytes"
	"errors"
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/core/events"
	"github.com/pgprotocol/pgp-chain/log"

	spv "github.com/elastos/Elastos.ELA.SPV/interface"
	elacom "github.com/elastos/Elastos.ELA/common"
	"github.com/elastos/Elastos.ELA/core/types/payload"
)

type NextTurnDPOSInfo struct {
	*payload.NextTurnDPOSInfo
}

var (
	nextTurnDposInfo *NextTurnDPOSInfo
	zero             = common.Hex2Bytes("000000000000000000000000000000000000000000000000000000000000000000")
)

const (
	CURRENT_PRODUCERS = "current_producers"
)

func GetTotalProducersCount() int {
	if nextTurnDposInfo == nil {
		return 0
	}
	count, err := SafeAdd(len(nextTurnDposInfo.CRPublicKeys), len(nextTurnDposInfo.DPOSPublicKeys))
	if err != nil {
		log.Error("SafeAdd error", "error", err)
		return 0
	}
	return count
}

func SpvIsWorkingHeight() bool {
	if nextTurnDposInfo != nil {
		return SpvService.GetBlockListener().BlockHeight() > nextTurnDposInfo.WorkingHeight
	}
	return false
}

func MainChainIsPowMode() bool {
	return consensusMode == spv.POW
}

func GetCRCPublicKeys(elaHeight uint64) [][]byte {
	crcArbiters, _, err := SpvService.GetArbiters(uint32(elaHeight))
	if err != nil {
		return nil
	}
	return crcArbiters
}

func GetProducers(elaHeight uint64) ([][]byte, int, error) {
	producers := make([][]byte, 0)
	totalCount := 0
	if SpvService == nil {
		return producers, totalCount, errors.New("spv is not start")
	}
	if GetCurrentConsensusMode() == spv.POW {
		producers = GetCurrentProducers()
		totalCount = len(producers)
		return producers, totalCount, nil
	}
	crcArbiters, normalArbitrs, err := SpvService.GetArbiters(uint32(elaHeight))
	if err != nil {
		return producers, totalCount, err
	}
	if IsOnlyCRConsensus {
		normalArbitrs = make([][]byte, 0)
	} else {
		crcArbiters = make([][]byte, 0)
	}

	for _, arbiter := range crcArbiters {
		if len(arbiter) > 0 && bytes.Compare(zero, arbiter) != 0 {
			producers = append(producers, arbiter)
		}
	}
	for _, arbiter := range normalArbitrs {
		if len(arbiter) > 0 && bytes.Compare(zero, arbiter) != 0 {
			producers = append(producers, arbiter)
		}
	}
	totalCount, err = SafeAdd(len(crcArbiters), len(normalArbitrs))
	if err != nil {
		return nil, totalCount, err
	}
	return producers, totalCount, nil
}

func GetSpvHeight() uint64 {
	if SpvService != nil && SpvService.GetBlockListener() != nil {
		header, err := SpvService.HeaderStore().GetBest()
		if err != nil {
			log.Error("SpvService getBest error", "error", err)
			return uint64(SpvService.GetBlockListener().BlockHeight())
		}
		return uint64(header.Height)
	}
	return 0
}

func GetWorkingHeight() uint32 {
	if nextTurnDposInfo != nil {
		return nextTurnDposInfo.WorkingHeight
	}
	return 0
}

func SetCurrentProducers(producers [][]byte) {
	if spvTransactiondb == nil {
		return
	}
	transactionDBMutex.Lock()
	defer transactionDBMutex.Unlock()
	b := new(bytes.Buffer)
	count := len(producers)
	if count == 0 {
		log.Error("not SetCurrentCRProducers crc arbitrator is empty")
		return
	}
	err := elacom.WriteVarUint(b, uint64(count))
	if err != nil {
		log.Error("[SetCurrentCRProducers]write count error", "error", err)
		return
	}

	for _, arbiter := range producers {
		if len(arbiter) > 0 && bytes.Compare(zero, arbiter) != 0 {
			err = elacom.WriteVarBytes(b, arbiter)
			if err != nil {
				log.Error("[SetCurrentCRProducers]WriteVarBytes error", "error", err)
				return
			}
		}
	}

	err = spvTransactiondb.Put([]byte(CURRENT_PRODUCERS), b.Bytes())
	if err != nil {
		log.Error("[setCurrentCRProducers] write db error", "error", err)
		return
	}
}

func GetCurrentProducers() [][]byte {
	if spvTransactiondb == nil {
		return nil
	}
	transactionDBMutex.Lock()
	defer transactionDBMutex.Unlock()
	b, err := spvTransactiondb.Get([]byte(CURRENT_PRODUCERS))
	if err != nil {
		log.Error("[GetCurrentCRProducers] read db error", "error", err)
		return nil
	}
	if b == nil {
		return nil
	}
	producers := make([][]byte, 0)
	reader := bytes.NewReader(b)
	count, err := elacom.ReadVarUint(reader, 0)
	for i := 0; i < int(count); i++ {
		arbiter, err := elacom.ReadVarBytes(reader, 33, "arbiter")
		if err != nil {
			log.Error("[GetCurrentCRProducers] read arbiter error", "error", err)
			return nil
		}
		if len(arbiter) > 0 && bytes.Compare(zero, arbiter) != 0 {
			producers = append(producers, arbiter)
		}
	}
	return producers
}

func BroadInitCurrentProducers() bool {
	if SpvService == nil {
		return false
	}
	err := SpvService.mux.Post(events.InitCurrentProducers{})
	if err != nil {
		return false
	}
	return true
}
