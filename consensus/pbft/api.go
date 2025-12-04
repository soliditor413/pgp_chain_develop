// Copyright (c) 2017-2019 The Elastos Foundation
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.
//

package pbft

import (
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/consensus"
	"github.com/pgprotocol/pgp-chain/dpos"
)

// API is a user facing RPC API to allow controlling the signer and voting
// mechanisms of the delegate-proof-of-stake scheme.
type API struct {
	chain consensus.ChainReader
	pbft  *Pbft
}

func (a *API) AnnounceDAddr() uint64 {
	if a.pbft.AnnounceDAddr() {
		return 1
	}
	return 0
}

func (a *API) GetArbiterPeersInfo() []peerInfo {
	return a.pbft.GetArbiterPeersInfo()
}

func (a *API) GetAllPeersInfo() []peerInfo {
	peers := a.pbft.GetAllArbiterPeersInfo()
	result := make([]peerInfo, 0)
	for _, peer := range peers {
		pid := peer.PID[:]
		result = append(result, peerInfo{
			NodePublicKey: common.Bytes2Hex(pid),
			IP:            peer.Addr,
			ConnState:     peer.State.String(),
			NodeVersion:   peer.NodeVersion,
		})
	}
	return result
}

func (a *API) GetActivePeers() []peerInfo {
	peers := a.pbft.network.GetActivePeers()
	result := make([]peerInfo, 0)
	for _, peer := range peers {
		pid := peer.PID()
		result = append(result, peerInfo{
			NodePublicKey: common.Bytes2Hex(pid[:]),
			IP:            peer.ToPeer().Addr(),
			ConnState:     peer.ToPeer().String(),
			NodeVersion:   peer.ToPeer().NodeVersion,
		})
	}
	return result
}

func (a *API) Dispatcher() *dpos.Dispatcher {
	return a.pbft.dispatcher
}

//func (a *API) Account() daccount.Account {
//	return a.pbft.account
//}

func (a *API) Network() *dpos.Network {
	return a.pbft.network
}

// GetProducerParticipationInfo returns participation information for a specific producer
// producerPubKeyHex: hex-encoded producer public key
func (a *API) GetProducerParticipationInfo(producerPubKeyHex string) *ParticipationInfo {
	if a.pbft.producerStats == nil {
		return nil
	}
	producerPubKey := common.Hex2Bytes(producerPubKeyHex)
	return a.pbft.producerStats.GetParticipationInfo(producerPubKey)
}

// GetProducerInactiveDuration returns how long a producer has been inactive (not participating in consensus)
// Returns the duration in seconds and a boolean indicating if the producer has never participated
// producerPubKeyHex: hex-encoded producer public key
func (a *API) GetProducerInactiveDuration(producerPubKeyHex string) (durationSeconds int64, neverParticipated bool) {
	if a.pbft.producerStats == nil {
		return 0, true
	}
	producerPubKey := common.Hex2Bytes(producerPubKeyHex)
	duration, neverParticipated := a.pbft.producerStats.GetInactiveDuration(producerPubKey)
	return int64(duration.Seconds()), neverParticipated
}

// GetAllProducersParticipationStats returns participation statistics for all known producers
func (a *API) GetAllProducersParticipationStats() map[string]*ParticipationInfo {
	if a.pbft.producerStats == nil {
		return make(map[string]*ParticipationInfo)
	}
	return a.pbft.producerStats.GetAllProducersStats()
}
