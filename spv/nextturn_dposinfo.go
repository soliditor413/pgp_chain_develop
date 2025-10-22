package spv

import (
	"bytes"
	"errors"
	"fmt"
	"math"

	spv "github.com/elastos/Elastos.ELA.SPV/interface"
	elacom "github.com/elastos/Elastos.ELA/common"
	"github.com/elastos/Elastos.ELA/core/types/payload"
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/core/events"
	"github.com/pgprotocol/pgp-chain/log"
	"sort"
)

var DefaultProducers = []string{
	"03244cbfdbee063261f9285fe028d8841cd5a4c4617fa285fa7a95dfedd20c3e5e",
	"03574acf5b9886eacdbfdeda46deabea107d1bfec11a400b0fdf1d79475fa74e01",
	"03830d4d3718e021289b3b0df1b0465c5cae4b403da403b1346dc42e7f0ae9461e",
	"03364106ea544e1c1175dea1ef487b5b56aa48ae680c303ea52631e31d6e5cd438",
	"022909c7d85c88d4d2a8091e279e5a800d2611a4f112019818fec4880a598b64e0",
	"0213d2ad8f4a167f12dd9056dd56c47b2d688277ce909c2bc64401fce6ae9290c3",
	"028d6bbd5965022e1e7263e65193342e43c2569f3b3c7bfa1b088122a7fb7fd925",
	"03b21a599807f516a3e7c00f1e402ce83e72482120f33d24577be5174117a94b7c",
	"0219accb8de9f2f2f5e12068b43552fa4a8118e223389e63585dfc10b33682133d",
	"033cb3eb2442862d37b729b9cafc310883078930e8554b1a0f95f70d50a0061454",
	"03e2283f3b5124bf55bbf4ea4734b493a3524d4b8d00c7b5107f52fcb235cf8069",
	"03361e8f72aed38135aa5ae96f68a95911de6710e2a0218c820844d52c5ee13304",
}

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
		sort.Slice(producers, func(i, j int) bool {
			return bytes.Compare(producers[i], producers[j]) < 0
		})
		totalCount = len(producers)
		return producers, totalCount, nil
	}
	if elaHeight == math.MaxUint64 { //defaults producers
		producers = GetDefaultProducers()
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
	sort.Slice(producers, func(i, j int) bool {
		return bytes.Compare(producers[i], producers[j]) < 0
	})
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

func GetDefaultProducers() [][]byte {
	defaultsProducers := DefaultProducers
	producers := make([][]byte, 0)
	for _, producer := range defaultsProducers {
		producers = append(producers, common.Hex2Bytes(producer))
	}
	fmt.Println("GetDefaultProducers =", producers)
	return producers
}

func GetCurrentProducers() [][]byte {
	if spvTransactiondb == nil {
		return nil
	}
	transactionDBMutex.Lock()
	defer transactionDBMutex.Unlock()
	b, err := spvTransactiondb.Get([]byte(CURRENT_PRODUCERS))
	if err != nil {
		spvHeight := GetSpvHeight()
		crcArbiters, normalArbitrs, err := SpvService.GetArbiters(uint32(spvHeight))
		log.Error("[GetCurrentCRProducers] read db error", "error", err)
		if IsOnlyCRConsensus {
			return crcArbiters
		} else {
			return normalArbitrs
		}
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
