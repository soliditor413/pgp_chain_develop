// Copyright (c) 2017-2019 The Elastos Foundation
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.
//
// Package pbft implements producer participation statistics tracking

package pbft

import (
	"bytes"
	"sync"
	"time"

	"github.com/elastos/Elastos.ELA/core/types/payload"
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/core/types"
	"github.com/pgprotocol/pgp-chain/log"
)

// ProducerStats tracks the participation statistics for each producer
type ProducerStats struct {
	mu                    sync.RWMutex
	lastParticipationTime map[string]time.Time // key: producer public key (hex), value: last participation time
	participationCount    map[string]uint64    // key: producer public key (hex), value: participation count
	lastBlockHeight       map[string]uint64    // key: producer public key (hex), value: last block height
}

// NewProducerStats creates a new ProducerStats instance
func NewProducerStats() *ProducerStats {
	return &ProducerStats{
		lastParticipationTime: make(map[string]time.Time),
		participationCount:    make(map[string]uint64),
		lastBlockHeight:       make(map[string]uint64),
	}
}

// RecordParticipation records a producer's participation in consensus
func (ps *ProducerStats) RecordParticipation(producerPubKey []byte, blockHeight uint64, blockTime uint64) {
	if len(producerPubKey) == 0 {
		return
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	producerKey := common.Bytes2Hex(producerPubKey)
	participationTime := time.Unix(int64(blockTime), 0)

	ps.lastParticipationTime[producerKey] = participationTime
	ps.participationCount[producerKey]++
	ps.lastBlockHeight[producerKey] = blockHeight

	log.Debug("Record producer participation",
		"producer", producerKey,
		"height", blockHeight,
		"time", participationTime,
		"count", ps.participationCount[producerKey])
}

// GetInactiveDuration returns how long a producer has been inactive (not participating in consensus)
// Returns the duration in seconds, and true if the producer has never participated
func (ps *ProducerStats) GetInactiveDuration(producerPubKey []byte) (duration time.Duration, neverParticipated bool) {
	if len(producerPubKey) == 0 {
		return 0, true
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	producerKey := common.Bytes2Hex(producerPubKey)
	lastTime, exists := ps.lastParticipationTime[producerKey]

	if !exists {
		return 0, true
	}

	duration = time.Since(lastTime)
	return duration, false
}

// GetParticipationInfo returns detailed participation information for a producer
type ParticipationInfo struct {
	ProducerPublicKey     string        `json:"producerPublicKey"`
	LastParticipationTime time.Time     `json:"lastParticipationTime"`
	InactiveDuration      time.Duration `json:"inactiveDuration"`
	ParticipationCount    uint64        `json:"participationCount"`
	LastBlockHeight       uint64        `json:"lastBlockHeight"`
	NeverParticipated     bool          `json:"neverParticipated"`
}

func (ps *ProducerStats) GetParticipationInfo(producerPubKey []byte) *ParticipationInfo {
	if len(producerPubKey) == 0 {
		return nil
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	producerKey := common.Bytes2Hex(producerPubKey)
	lastTime, exists := ps.lastParticipationTime[producerKey]

	info := &ParticipationInfo{
		ProducerPublicKey: producerKey,
		NeverParticipated: !exists,
	}

	if exists {
		info.LastParticipationTime = lastTime
		info.InactiveDuration = time.Since(lastTime)
		info.ParticipationCount = ps.participationCount[producerKey]
		info.LastBlockHeight = ps.lastBlockHeight[producerKey]
	}

	return info
}

// GetAllProducersStats returns participation statistics for all known producers
func (ps *ProducerStats) GetAllProducersStats() map[string]*ParticipationInfo {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make(map[string]*ParticipationInfo)
	now := time.Now()

	for producerKey := range ps.lastParticipationTime {
		lastTime := ps.lastParticipationTime[producerKey]
		result[producerKey] = &ParticipationInfo{
			ProducerPublicKey:     producerKey,
			LastParticipationTime: lastTime,
			InactiveDuration:      now.Sub(lastTime),
			ParticipationCount:    ps.participationCount[producerKey],
			LastBlockHeight:       ps.lastBlockHeight[producerKey],
			NeverParticipated:     false,
		}
	}

	return result
}

// extractProducerFromBlock extracts the producer public key from a block's confirm
func extractProducerFromBlock(block *types.Block) ([]byte, error) {
	if block == nil || len(block.Extra()) == 0 {
		return nil, nil
	}

	var confirm payload.Confirm
	err := confirm.Deserialize(bytes.NewReader(block.Extra()))
	if err != nil {
		return nil, err
	}

	// The producer is the sponsor of the proposal
	return confirm.Proposal.Sponsor, nil
}
