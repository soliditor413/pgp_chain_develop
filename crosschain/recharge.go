package crosschain

import (
	"github.com/pgprotocol/pgp-chain/core/types"
	"github.com/pgprotocol/pgp-chain/spv"
)

func IsRechargeTx(tx *types.Transaction) bool {
	if tx == nil || tx.To() == nil {
		return false
	}
	//var empty common.Address
	//if *tx.To() == empty {
	//	if len(tx.Data()) == 32 {
	//		return true
	//	}
	//	rawTxid, _, _, _ := spv.IsSmallCrossTxByData(tx.Data())
	//	if rawTxid != "" {
	//		return true
	//	}
	//}
	//return false
	return spv.IsRechargeTx(tx.Data(), tx.To())
}
