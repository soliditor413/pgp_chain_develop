// Copyright (c) 2017-2019 The Elastos Foundation
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.
//
// Package pbft implements producer participation statistics tracking

package pbft

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/elastos/Elastos.ELA/core/types/payload"
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/core/types"
	"github.com/pgprotocol/pgp-chain/ethdb"
	"github.com/pgprotocol/pgp-chain/ethdb/leveldb"
	"github.com/pgprotocol/pgp-chain/log"
)

const (
	// InactiveThreshold is the number of consecutive blocks a producer must miss to be marked as inactive
	InactiveThreshold uint64 = 20
	// blacklistOpMinInterval 同一 producer 的添加/移除黑名单操作最小间隔，避免重复发送
	blacklistOpMinInterval = 5 * time.Second
	// producerStatsDBName is the database name for storing producer statistics
	producerStatsDBName    = "producer_stats"
	blacklistDBPrefix      = "blacklist:"
	blacklistScanHeightKey = "blacklistScanHeight"
)

// dbInterface combines the interfaces we need for persistence
type dbInterface interface {
	ethdb.KeyValueWriter
	ethdb.KeyValueReader
	ethdb.Iteratee
}

// ProducerStats tracks the participation statistics for each producer
type ProducerStats struct {
	mu                       sync.RWMutex
	db                       dbInterface         // Database for persistence
	dataDir                  string              // Data directory path
	lastParticipationTime    map[string]uint64   // key: producer public key (hex), value: last participation time (unix seconds)
	lastBlockHeight          map[string]uint64   // key: producer public key (hex), value: last block height
	consecutiveMissedBlocks  map[string]uint64   // key: producer public key (hex), value: consecutive missed blocks
	confirmedBlacklist       map[string]struct{} // key: producer public key (hex)
	blacklistScannedHeight   uint64
	lastProcessedBlockHeight uint64    // last processed block height to avoid duplicate processing
	currentBlockTime         time.Time // last seen block time (from chain)
	blacklistOracle          BlacklistOracle
	// 同一 producer 添加/移除黑名单操作节流：上次操作时间
	muOpTime            sync.Mutex
	lastBlacklistOpTime map[string]time.Time
}

type blacklistVoteTarget struct {
	producerKey    string
	lastSealHeight uint64
	currentHeight  uint64
}

// NewProducerStats creates a new ProducerStats instance
// dataDir: the data directory for storing the database, empty string means no persistence
func NewProducerStats(dataDir string) (*ProducerStats, error) {
	ps := &ProducerStats{
		dataDir:                  dataDir,
		lastParticipationTime:    make(map[string]uint64),
		lastBlockHeight:          make(map[string]uint64),
		consecutiveMissedBlocks:  make(map[string]uint64),
		confirmedBlacklist:       make(map[string]struct{}),
		lastProcessedBlockHeight: 0,
		lastBlacklistOpTime:      make(map[string]time.Time),
	}

	// Open database for persistence
	if dataDir != "" {
		dbPath := filepath.Join(dataDir, producerStatsDBName)
		db, err := leveldb.New(dbPath, 16, 16, "pbft/producer_stats")
		if err != nil {
			log.Error("Failed to open producer stats database", "path", dbPath, "error", err)
			return nil, err
		}
		ps.db = db

		// Load data from database
		if err := ps.loadFromDB(); err != nil {
			log.Error("Failed to load producer stats from database", "error", err)
			// Continue with empty stats if load fails
		}
	}

	return ps, nil
}

// ConfigureBlacklist sets the oracle used for blacklist votes.
func (ps *ProducerStats) ConfigureBlacklist(oracle BlacklistOracle) {
	ps.mu.Lock()
	oldOracle := ps.blacklistOracle
	ps.blacklistOracle = oracle
	ps.mu.Unlock()

	if oldOracle != nil {
		oldOracle.StopListener()
	}
	if oracle != nil {
		if err := oracle.StartListener(ps.onBlacklistConfirmed, ps.onBlacklistRemoved, ps.getBlacklistScannedHeight, ps.setBlacklistScannedHeight); err != nil {
			log.Error("Failed to start blacklist listener", "error", err)
		}
	}
}

// Close closes the database connection
func (ps *ProducerStats) Close() error {
	ps.mu.Lock()
	oracle := ps.blacklistOracle
	ps.blacklistOracle = nil
	ps.mu.Unlock()
	if oracle != nil {
		oracle.StopListener()
	}
	if ps.db != nil {
		if closer, ok := ps.db.(interface{ Close() error }); ok {
			return closer.Close()
		}
	}
	return nil
}

// RecordParticipation records a producer's participation in consensus
func (ps *ProducerStats) RecordParticipation(producerPubKey []byte, blockHeight uint64, blockTime uint64) {
	if len(producerPubKey) == 0 {
		return
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()

	producerKey := common.Bytes2Hex(producerPubKey)

	if ps.lastBlockHeight[producerKey] == blockHeight {
		return
	}
	ps.lastParticipationTime[producerKey] = blockTime
	ps.lastBlockHeight[producerKey] = blockHeight

	// Reset consecutive missed blocks when producer participates
	ps.consecutiveMissedBlocks[producerKey] = 0

	log.Info("Record producer participation",
		"producer", producerKey,
		"height", blockHeight,
		"time", time.Unix(int64(blockTime), 0))

	// Save to database
	ps.saveProducerToDB(producerKey)
}

// GetParticipationInfo returns detailed participation information for a producer
type ParticipationInfo struct {
	ProducerPublicKey     string        `json:"producerPublicKey"`
	LastParticipationTime uint64        `json:"lastParticipationTime"` // unix seconds
	InactiveDuration      time.Duration `json:"inactiveDuration"`
	ParticipationCount    uint64        `json:"participationCount"`
	LastBlockHeight       uint64        `json:"lastBlockHeight"`
	NeverParticipated     bool          `json:"neverParticipated"`
}

// UpdateBlockHeight updates the block height/time and checks for inactive producers
// This should be called when a new block is inserted, with the list of current producers
// blockTime is seconds since epoch from the block header
func (ps *ProducerStats) UpdateBlockHeight(blockHeight uint64, blockTime uint64, currentProducers [][]byte) {
	// 未在 confirmedBlacklist 且已达 InactiveThreshold 的 producer，先收集；释放锁后按是否过期拆成 add / remove
	var inactiveNotConfirmed []blacklistVoteTarget
	ps.mu.Lock()

	// Avoid processing the same block height multiple times
	if blockHeight <= ps.lastProcessedBlockHeight {
		ps.mu.Unlock()
		return
	}
	log.Info("------------ >>>>>>> UpdateBlockHeight", "blockHeight ", blockHeight, "blockTime:", blockTime)
	// Update current block height
	ps.lastProcessedBlockHeight = blockHeight
	ps.currentBlockTime = time.Unix(int64(blockTime), 0)

	// Create a set of current producers for quick lookup
	producerSet := make(map[string]bool)
	for _, producer := range currentProducers {
		producerKey := common.Bytes2Hex(producer)
		producerSet[producerKey] = true
		if _, exists := ps.lastParticipationTime[producerKey]; !exists {
			// New producer, initialize with 0 missed blocks
			ps.consecutiveMissedBlocks[producerKey] = 0
			ps.lastParticipationTime[producerKey] = 0
		}
	}

	// Update consecutive missed blocks for all known producers
	// Only track producers that are in the current producer list
	for producerKey := range ps.lastParticipationTime {
		// Only track if this producer is in the current producer list
		if !producerSet[producerKey] {
			continue
		}

		// Check if this producer participated in the last block.
		// Producers that have never sealed a block keep lastHeight=0 and should still accumulate misses.
		if lastHeight := ps.lastBlockHeight[producerKey]; lastHeight < blockHeight {
			ps.consecutiveMissedBlocks[producerKey]++
			log.Info("Missed Blocks ", "producer:", producerKey, "count:", ps.consecutiveMissedBlocks[producerKey], " InactiveThreshold:", InactiveThreshold)
			// 未在 confirmedBlacklist 且已达 InactiveThreshold：收集后根据是否过期决定 add 或 remove
			if ps.consecutiveMissedBlocks[producerKey] >= InactiveThreshold {
				if _, exists := ps.confirmedBlacklist[producerKey]; !exists {
					inactiveNotConfirmed = append(inactiveNotConfirmed, blacklistVoteTarget{
						producerKey:    producerKey,
						lastSealHeight: lastHeight,
						currentHeight:  blockHeight,
					})
					log.Warn("Producer marked as inactive (cannot participate in consensus)",
						"producer", producerKey,
						"consecutiveMissedBlocks", ps.consecutiveMissedBlocks[producerKey],
						"height", blockHeight)
				}
			}
			ps.saveProducerToDB(producerKey)
		}

	}

	// Save current block height
	if ps.db != nil {
		key := []byte("currentBlockHeight")
		value := make([]byte, 8)
		binary.BigEndian.PutUint64(value, blockHeight)
		if err := ps.db.Put(key, value); err != nil {
			log.Error("Failed to save current block height", "error", err)
		}
	}
	ps.mu.Unlock()

	// 释放锁后按是否过期拆分：已过期则加入 remove 列表并尝试移除投票，未过期则加入黑名单
	var addTargets []blacklistVoteTarget
	var removeTargets []blacklistVoteTarget
	if ps.blacklistOracle != nil {
		for _, t := range inactiveNotConfirmed {
			expired, err := ps.blacklistOracle.IsExpired(common.Hex2Bytes(t.producerKey))
			if err != nil {
				log.Error("Query blacklist expired status failed", "producer", t.producerKey, "error", err)
				continue
			}
			if expired {
				log.Info("Producer is expired, add to remove targets", "producer", t.producerKey)
				removeTargets = append(removeTargets, t)
			} else {
				log.Info("Producer is not expired, add to add targets", "producer", t.producerKey)
				addTargets = append(addTargets, t)
			}
		}
	} else {
		addTargets = inactiveNotConfirmed
	}
	// 先按 5 秒节流过滤，再按合约 hasAddVoted 过滤已投过票的，最后统一用 SubmitBlacklistVotesBatch 提交（内部用 nonce, nonce+1, ...）
	allowedAddTargets := ps.filterAddTargetsByThrottle(addTargets)
	if len(allowedAddTargets) > 0 && ps.blacklistOracle != nil {
		var batchTargets []blacklistVoteTarget
		for _, t := range allowedAddTargets {
			voted, err := ps.blacklistOracle.HasAddVoted(common.Hex2Bytes(t.producerKey))
			if err != nil {
				log.Error("Query hasAddVoted failed", "producer", t.producerKey, "error", err)
				continue
			}
			if voted {
				log.Info("Producer has already submitted add vote", "producer", t.producerKey)
				continue
			}
			batchTargets = append(batchTargets, t)
		}
		if len(batchTargets) > 0 {
			keys := make([]string, len(batchTargets))
			heights := make([]uint64, len(batchTargets))
			for i, t := range batchTargets {
				keys[i] = t.producerKey
				heights[i] = t.lastSealHeight
			}
			if err := ps.blacklistOracle.SubmitBlacklistVotesBatch(keys, heights); err != nil {
				log.Error("Submit blacklist votes batch failed", "error", err)
			} else {
				ps.recordBlacklistOpTime(keys)
			}
		}
	}
	ps.trySubmitRemoveBlacklistVotes(removeTargets)
}

// GetConsecutiveMissedBlocks returns the number of consecutive blocks a producer has missed
func (ps *ProducerStats) GetConsecutiveMissedBlocks(producerPubKey []byte) uint64 {
	if len(producerPubKey) == 0 {
		return 0
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	producerKey := common.Bytes2Hex(producerPubKey)
	return ps.consecutiveMissedBlocks[producerKey]
}

// filterAddTargetsByThrottle 过滤掉 5 秒内已有黑名单操作的 producer，避免重复发送。
func (ps *ProducerStats) filterAddTargetsByThrottle(targets []blacklistVoteTarget) []blacklistVoteTarget {
	ps.muOpTime.Lock()
	defer ps.muOpTime.Unlock()
	out := make([]blacklistVoteTarget, 0, len(targets))
	for _, t := range targets {
		if last, ok := ps.lastBlacklistOpTime[t.producerKey]; ok && time.Since(last) < blacklistOpMinInterval {
			continue
		}
		out = append(out, t)
	}
	return out
}

// recordBlacklistOpTime 记录批量添加黑名单后每个 producer 的操作时间（用于 5 秒节流）。
func (ps *ProducerStats) recordBlacklistOpTime(producerKeys []string) {
	now := time.Now()
	ps.muOpTime.Lock()
	defer ps.muOpTime.Unlock()
	for _, k := range producerKeys {
		ps.lastBlacklistOpTime[k] = now
	}
}

// addToBlacklist adds a producer to the blacklist
func (ps *ProducerStats) addToBlacklist(producerKey string, lastSealHeight uint64, currentHeight uint64) {
	ps.muOpTime.Lock()
	if last, ok := ps.lastBlacklistOpTime[producerKey]; ok && time.Since(last) < blacklistOpMinInterval {
		ps.muOpTime.Unlock()
		log.Debug("Skip add blacklist: within min interval", "producer", producerKey)
		return
	}
	ps.muOpTime.Unlock()

	// Submit blacklist vote to contract (best-effort)
	if err := ps.submitBlacklistVote(producerKey, lastSealHeight); err != nil {
		log.Error("Submit blacklist vote failed",
			"producer", producerKey,
			"lastSealHeight", lastSealHeight,
			"error", err)
		return
	}
	ps.muOpTime.Lock()
	ps.lastBlacklistOpTime[producerKey] = time.Now()
	ps.muOpTime.Unlock()
	log.Warn("Producer marked as inactive and submitted blacklist vote",
		"producer", producerKey,
		"lastSealHeight", lastSealHeight,
		"height", currentHeight)
}

func (ps *ProducerStats) submitBlacklistVote(producerKey string, lastSealBlockHeight uint64) error {
	if ps.blacklistOracle == nil {
		return nil
	}
	targetPubKey := common.Hex2Bytes(producerKey)
	if voted, err := ps.blacklistOracle.HasAddVoted(targetPubKey); voted || err != nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("already submitted add vote for: %s", producerKey)
	}
	if expired, err := ps.blacklistOracle.IsExpired(targetPubKey); expired || err != nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("blacklist expired for: %s", producerKey)
	}
	if res, err := ps.blacklistOracle.IsBlacklisted(targetPubKey); res == true || err != nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("blacklisted : %s", producerKey)
	}
	return ps.blacklistOracle.SubmitBlacklistVote(producerKey, lastSealBlockHeight)
}

// trySubmitRemoveBlacklistVotes 对 snapshot 中的目标以及可选的 extraTargets 尝试提交移除黑名单投票（仅对已过期的提交）。
// 通过条件的目标统一用 SubmitRemoveBlacklistVotesBatch 批量提交（nonce, nonce+1, ...），避免多笔同 nonce。
// extraTargets 用于本次未在 confirmedBlacklist 但已过期的 producer，由 UpdateBlockHeight 传入。
func (ps *ProducerStats) trySubmitRemoveBlacklistVotes(extraTargets []blacklistVoteTarget) {
	if ps.blacklistOracle == nil {
		return
	}
	targets := ps.snapshotRemoveVoteTargets()
	seen := make(map[string]bool)
	for _, t := range targets {
		seen[t.producerKey] = true
	}
	for _, t := range extraTargets {
		if !seen[t.producerKey] {
			seen[t.producerKey] = true
			targets = append(targets, t)
		}
	}
	var toRemove []string
	for _, target := range targets {
		producerKey := target.producerKey
		targetPubKey := common.Hex2Bytes(producerKey)
		expired, err := ps.blacklistOracle.IsExpired(targetPubKey)
		if err != nil {
			log.Error("Query blacklist expired status failed", "producer", producerKey, "error ", err)
			continue
		}
		if !expired {
			continue
		}
		removeVoted, err := ps.blacklistOracle.HasRemoveVoted(targetPubKey)
		if err != nil {
			log.Error("Query hasRemoveVoted failed", "producer", producerKey, "error", err)
			continue
		}
		if removeVoted {
			continue
		}
		ps.muOpTime.Lock()
		if last, ok := ps.lastBlacklistOpTime[producerKey]; ok && time.Since(last) < blacklistOpMinInterval {
			ps.muOpTime.Unlock()
			log.Debug("Skip remove blacklist vote: within min interval", "producer", producerKey)
			continue
		}
		ps.muOpTime.Unlock()
		toRemove = append(toRemove, producerKey)
	}
	// 提交前再按合约 hasRemoveVoted 过滤一次，只提交未投过移除票的
	if len(toRemove) > 0 {
		var batchRemove []string
		for _, producerKey := range toRemove {
			voted, err := ps.blacklistOracle.HasRemoveVoted(common.Hex2Bytes(producerKey))
			if err != nil {
				log.Error("Query hasRemoveVoted failed before batch", "producer", producerKey, "error", err)
				continue
			}
			if voted {
				continue
			}
			batchRemove = append(batchRemove, producerKey)
		}
		if len(batchRemove) > 0 {
			if err := ps.blacklistOracle.SubmitRemoveBlacklistVotesBatch(batchRemove); err != nil {
				log.Error("Submit remove blacklist votes batch failed", "error", err)
			} else {
				ps.recordBlacklistOpTime(batchRemove)
			}
		}
	}
}

func (ps *ProducerStats) onBlacklistConfirmed(dposPublicKey []byte) {
	if len(dposPublicKey) == 0 {
		return
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()

	producerKey := common.Bytes2Hex(dposPublicKey)
	if _, exists := ps.confirmedBlacklist[producerKey]; exists {
		return
	}
	ps.confirmedBlacklist[producerKey] = struct{}{}
	ps.saveConfirmedBlacklistToDB(producerKey)
	log.Info("Producer confirmed in blacklist", "producer", producerKey)
}

func (ps *ProducerStats) onBlacklistRemoved(dposPublicKey []byte) {
	if len(dposPublicKey) == 0 {
		return
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()

	producerKey := common.Bytes2Hex(dposPublicKey)
	ps.deleteConfirmedBlacklist(producerKey)
	ps.consecutiveMissedBlocks[producerKey] = 0
}

func (ps *ProducerStats) getNow() time.Time {
	ps.mu.RLock()
	now := ps.currentBlockTime
	ps.mu.RUnlock()
	if now.IsZero() {
		return time.Now()
	}
	return now
}

// ProducerStatsData represents the serialized data for a producer
type ProducerStatsData struct {
	LastParticipationTime   int64  `json:"lastParticipationTime"`
	ParticipationCount      uint64 `json:"participationCount"`
	LastBlockHeight         uint64 `json:"lastBlockHeight"`
	ConsecutiveMissedBlocks uint64 `json:"consecutiveMissedBlocks"`
}

// saveProducerToDB saves a producer's statistics to the database
func (ps *ProducerStats) saveProducerToDB(producerKey string) {
	if ps.db == nil {
		return
	}
	data := ProducerStatsData{
		LastParticipationTime:   int64(ps.lastParticipationTime[producerKey]),
		LastBlockHeight:         ps.lastBlockHeight[producerKey],
		ConsecutiveMissedBlocks: ps.consecutiveMissedBlocks[producerKey],
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Error("Failed to marshal producer stats", "producer", producerKey, "error", err)
		return
	}

	key := []byte("producer:" + producerKey)
	if err := ps.db.Put(key, jsonData); err != nil {
		log.Error("Failed to save producer stats to database", "producer", producerKey, "error", err)
	}
}

// loadFromDB loads all producer statistics from the database
func (ps *ProducerStats) loadFromDB() error {
	if ps.db == nil {
		return nil
	}

	// Load current block height
	key := []byte("currentBlockHeight")
	if value, err := ps.db.Get(key); err == nil && len(value) == 8 {
		ps.lastProcessedBlockHeight = binary.BigEndian.Uint64(value)
	}
	if value, err := ps.db.Get([]byte(blacklistScanHeightKey)); err == nil && len(value) == 8 {
		ps.blacklistScannedHeight = binary.BigEndian.Uint64(value)
	}

	// Iterate through all keys with prefix "producer:"
	prefix := []byte("producer:")
	it := ps.db.NewIteratorWithPrefix(prefix)
	defer it.Release()

	count := 0
	for it.Next() {
		key := it.Key()
		value := it.Value()

		// Extract producer key from "producer:xxxxx"
		if len(key) <= len(prefix) {
			continue
		}
		producerKey := string(key[len(prefix):])

		var data ProducerStatsData
		if err := json.Unmarshal(value, &data); err != nil {
			log.Error("Failed to unmarshal producer stats", "producer", producerKey, "error", err)
			continue
		}

		// Restore producer statistics
		ps.lastParticipationTime[producerKey] = uint64(data.LastParticipationTime)
		ps.lastBlockHeight[producerKey] = data.LastBlockHeight
		ps.consecutiveMissedBlocks[producerKey] = data.ConsecutiveMissedBlocks

		count++
	}

	log.Info("Loaded producer statistics from database",
		"producerCount", count,
		"currentBlockHeight", ps.lastProcessedBlockHeight)

	blacklistPrefix := []byte(blacklistDBPrefix)
	blacklistIt := ps.db.NewIteratorWithPrefix(blacklistPrefix)
	defer blacklistIt.Release()
	for blacklistIt.Next() {
		key := blacklistIt.Key()
		if len(key) <= len(blacklistPrefix) {
			continue
		}
		producerKey := string(key[len(blacklistPrefix):])
		ps.confirmedBlacklist[producerKey] = struct{}{}
	}
	if err := blacklistIt.Error(); err != nil {
		return err
	}
	return it.Error()
}

func (ps *ProducerStats) saveConfirmedBlacklistToDB(producerKey string) {
	if ps.db == nil {
		return
	}
	if err := ps.db.Put([]byte(blacklistDBPrefix+producerKey), []byte{1}); err != nil {
		log.Error("Failed to save confirmed blacklist", "producer", producerKey, "error", err)
	}
}

func (ps *ProducerStats) deleteConfirmedBlacklist(producerKey string) {
	if _, exists := ps.confirmedBlacklist[producerKey]; !exists {
		return
	}
	delete(ps.confirmedBlacklist, producerKey)
	if ps.db == nil {
		return
	}
	if err := ps.db.Delete([]byte(blacklistDBPrefix + producerKey)); err != nil {
		log.Error("Failed to delete confirmed blacklist", "producer", producerKey, "error", err)
	}
}

func (ps *ProducerStats) snapshotRemoveVoteTargets() []blacklistVoteTarget {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	targets := make([]blacklistVoteTarget, 0, len(ps.confirmedBlacklist))
	for producerKey := range ps.confirmedBlacklist {
		targets = append(targets, blacklistVoteTarget{
			producerKey:    producerKey,
			lastSealHeight: ps.lastBlockHeight[producerKey],
		})
	}
	return targets
}

func (ps *ProducerStats) getBlacklistScannedHeight() uint64 {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.blacklistScannedHeight
}

func (ps *ProducerStats) setBlacklistScannedHeight(height uint64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if height <= ps.blacklistScannedHeight {
		return
	}
	ps.blacklistScannedHeight = height
	if ps.db == nil {
		return
	}
	value := make([]byte, 8)
	binary.BigEndian.PutUint64(value, height)
	if err := ps.db.Put([]byte(blacklistScanHeightKey), value); err != nil {
		log.Error("Failed to save blacklist scanned height", "height", height, "error", err)
	}
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
