package crosschain

import (
	"github.com/pgprotocol/pgp-chain/core/types"
	"github.com/pgprotocol/pgp-chain/spv"
)

func IsSystemTx(tx *types.Transaction) bool {
	if tx == nil || tx.To() == nil {
		return false
	}
	return spv.IsRechargeTx(tx.Data(), tx.To()) || spv.IsRefundWithdrawTx(tx.Data(), tx.To())
}
