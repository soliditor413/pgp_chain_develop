package eth

import (
	"github.com/pgprotocol/pgp-chain/consensus/pbft"
	"github.com/pgprotocol/pgp-chain/log"
	"github.com/pgprotocol/pgp-chain/spv"

	"time"
)

type BposNetwork struct {
	engine *pbft.Pbft
	timer  *time.Timer
}

func NewBposNetwork(engine *pbft.Pbft) *BposNetwork {
	return &BposNetwork{
		engine: engine,
	}
}

func (n *BposNetwork) Start() {
	go n.turnToDposConsensus()
}

func (n *BposNetwork) turnToDposConsensus() {
	res := spv.BroadInitCurrentProducers()
	if !res {
		time.Sleep(time.Second * 10)
		n.turnToDposConsensus()
		return
	}
	if n.timer == nil {
		n.timer = time.NewTimer(time.Second * 60)
	} else {
		n.timer.Reset(time.Second * 60)
	}
	spv.IsOnlyCRConsensus = false

	go func() {
		for {
			select {
			case <-n.timer.C:
				currentHeader := n.engine.GetBlockChain().CurrentHeader()
				if uint64(time.Now().Unix())-currentHeader.Time > 60 {
					log.Info("check direct network")
					n.checkNetwork()
				}
				n.timer.Reset(time.Second * 60)
			}
		}
	}()
}

// checkNetwork checks the number of connected peers, if the number of unconnected peers exceeds 1/3 of the total number of peers, it will turn to CR Consensus.
func (n *BposNetwork) checkNetwork() {
	peers := n.engine.GetArbiterPeersInfo()
	total := n.engine.GetTotalArbitersCount()
	noneConnecttedCount := 0
	for _, peer := range peers {
		if peer.ConnState != "2WayConnection" {
			noneConnecttedCount++
		}
	}
	if noneConnecttedCount >= total/3 { //turn to CR Consensus
		n.timer.Stop()
		n.turnToCRCConsensus()
	}
}

func (n *BposNetwork) turnToCRCConsensus() {
	res := spv.BroadInitCurrentProducers()
	if !res {
		time.Sleep(time.Second * 10)
		n.turnToCRCConsensus()
		return
	}
	spv.IsOnlyCRConsensus = true
	go func() {
		select {
		case <-time.After(time.Second * 3600):
			n.turnToDposConsensus()
		}
	}()
}
