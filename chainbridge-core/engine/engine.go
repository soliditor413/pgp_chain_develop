package engine

import (
	"github.com/pgprotocol/pgp-chain/chainbridge-core/crypto"

	"github.com/elastos/Elastos.ELA/dpos/p2p/peer"
	"github.com/elastos/Elastos.ELA/p2p"
)

type ESCEngine interface {
	IsOnduty() bool
	SendMsgProposal(proposalMsg p2p.Message)
	SendMsgToPeer(proposalMsg p2p.Message, pid peer.PID)
	SignData(data []byte) []byte
	DecryptArbiter(cipher []byte) (arbiter []byte, err error)
	GetProducer() []byte
	GetBridgeArbiters() crypto.Keypair
	GetTotalArbitersCount() int
	IsSyncFinished() bool
}
