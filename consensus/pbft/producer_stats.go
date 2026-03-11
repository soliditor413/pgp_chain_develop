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
	// producerStatsDBName is the database name for storing producer statistics
	producerStatsDBName = "producer_stats"
	// CleanupThresholdDays is the number of days after which inactive producers not in current list can be cleaned up
	CleanupThresholdDays = 30
	// CleanupIntervalBlocks is the number of blocks between cleanup operations
	CleanupIntervalBlocks uint64 = 10000
	// BlacklistRemovalAfter defines when to start submitting remove blacklist votes
	BlacklistRemovalAfter = 10 * time.Minute // 7 * 24 * time.Hour
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
	db                       dbInterface       // Database for persistence
	dataDir                  string            // Data directory path
	lastParticipationTime    map[string]uint64 // key: producer public key (hex), value: last participation time (unix seconds)
	participationCount       map[string]uint64 // key: producer public key (hex), value: participation count
	lastBlockHeight          map[string]uint64 // key: producer public key (hex), value: last block height
	consecutiveMissedBlocks  map[string]uint64 // key: producer public key (hex), value: consecutive missed blocks
	isInactive               map[string]bool   // key: producer public key (hex), value: true if inactive (cannot participate in consensus)
	removeVoteSubmitted      map[string]bool   // key: producer public key (hex), value: true if remove vote already submitted
	currentBlockHeight       uint64            // current block height for tracking
	lastProcessedBlockHeight uint64            // last processed block height to avoid duplicate processing
	lastCleanupBlockHeight   uint64            // last block height when cleanup was performed
	currentBlockTime         time.Time         // last seen block time (from chain)
	blacklistOracle          BlacklistOracle
}

// NewProducerStats creates a new ProducerStats instance
// dataDir: the data directory for storing the database, empty string means no persistence
func NewProducerStats(dataDir string) (*ProducerStats, error) {
	ps := &ProducerStats{
		dataDir:                  dataDir,
		lastParticipationTime:    make(map[string]uint64),
		participationCount:       make(map[string]uint64),
		lastBlockHeight:          make(map[string]uint64),
		consecutiveMissedBlocks:  make(map[string]uint64),
		isInactive:               make(map[string]bool),
		removeVoteSubmitted:      make(map[string]bool),
		currentBlockHeight:       0,
		lastProcessedBlockHeight: 0,
		lastCleanupBlockHeight:   0,
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
	defer ps.mu.Unlock()
	ps.blacklistOracle = oracle
}

// Close closes the database connection
func (ps *ProducerStats) Close() error {
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
	participationTime := blockTime
	if participationTime == 0 {
		participationTime = uint64(time.Now().Unix())
	}

	ps.lastParticipationTime[producerKey] = blockTime
	ps.participationCount[producerKey]++
	ps.lastBlockHeight[producerKey] = blockHeight

	// Reset consecutive missed blocks when producer participates
	ps.consecutiveMissedBlocks[producerKey] = 0

	// Remove inactive status if producer participates again
	if ps.isInactive[producerKey] {
		ps.isInactive[producerKey] = false
		ps.removeVoteSubmitted[producerKey] = false
		log.Info("Producer removed from inactive status due to participation",
			"producer", producerKey,
			"height", blockHeight)
	}

	log.Info("Record producer participation",
		"producer", producerKey,
		"height", blockHeight,
		"time", time.Unix(int64(participationTime), 0),
		"count", ps.participationCount[producerKey])

	// Save to database
	ps.saveProducerToDB(producerKey)
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
	lastTimeSec, exists := ps.lastParticipationTime[producerKey]

	if !exists {
		return 0, true
	}

	nowSec := ps.getNow().Unix()
	if nowSec < 0 || lastTimeSec > uint64(nowSec) {
		return 0, false
	}
	duration = time.Duration(nowSec-int64(lastTimeSec)) * time.Second
	return duration, false
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

func (ps *ProducerStats) GetParticipationInfo(producerPubKey []byte) *ParticipationInfo {
	if len(producerPubKey) == 0 {
		return nil
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	producerKey := common.Bytes2Hex(producerPubKey)
	lastTimeSec, exists := ps.lastParticipationTime[producerKey]

	info := &ParticipationInfo{
		ProducerPublicKey: producerKey,
		NeverParticipated: !exists,
	}

	if exists {
		info.LastParticipationTime = lastTimeSec
		nowSec := ps.getNow().Unix()
		if nowSec >= 0 && lastTimeSec <= uint64(nowSec) {
			info.InactiveDuration = time.Duration(int64(nowSec)-int64(lastTimeSec)) * time.Second
		}
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
	nowSec := ps.getNow().Unix()

	for producerKey := range ps.lastParticipationTime {
		lastSec := ps.lastParticipationTime[producerKey]
		result[producerKey] = &ParticipationInfo{
			ProducerPublicKey:     producerKey,
			LastParticipationTime: lastSec,
			InactiveDuration:      durationSinceSec(nowSec, lastSec),
			ParticipationCount:    ps.participationCount[producerKey],
			LastBlockHeight:       ps.lastBlockHeight[producerKey],
			NeverParticipated:     false,
		}
	}

	return result
}

// UpdateBlockHeight updates the block height/time and checks for inactive producers
// This should be called when a new block is inserted, with the list of current producers
// blockTime is seconds since epoch from the block header
func (ps *ProducerStats) UpdateBlockHeight(blockHeight uint64, blockTime uint64, currentProducers [][]byte) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Avoid processing the same block height multiple times
	if blockHeight <= ps.lastProcessedBlockHeight {
		return
	}
	log.Info("------------ >>>>>>> UpdateBlockHeight", "blockHeight ", blockHeight, "blockTime:", blockTime)
	// Update current block height
	ps.currentBlockHeight = blockHeight
	ps.lastProcessedBlockHeight = blockHeight
	ps.currentBlockTime = time.Unix(int64(blockTime), 0)

	// Create a set of current producers for quick lookup
	producerSet := make(map[string]bool)
	for _, producer := range currentProducers {
		producerKey := common.Bytes2Hex(producer)
		fmt.Println("producerKey ", producerKey)
		producerSet[producerKey] = true
	}

	// Update consecutive missed blocks for all known producers
	// Only track producers that are in the current producer list
	needsSave := false
	for producerKey := range ps.lastParticipationTime {
		// Only track if this producer is in the current producer list
		if !producerSet[producerKey] {
			log.Warn("is not in current producers ", "producer:", producerKey)
			continue
		}

		// Check if this producer participated in the last block
		// If lastBlockHeight is less than current block height, they missed this block
		if ps.lastBlockHeight[producerKey] < blockHeight {
			ps.consecutiveMissedBlocks[producerKey]++
			log.Info("Missed Blocks ", "producer:", producerKey, "count:", ps.consecutiveMissedBlocks[producerKey])
			// Check if should be marked as inactive
			if ps.consecutiveMissedBlocks[producerKey] >= InactiveThreshold {
				if !ps.isInactive[producerKey] {
					ps.isInactive[producerKey] = true
					ps.removeVoteSubmitted[producerKey] = false

					// Add to blacklist (permanent record)
					ps.addToBlacklist(producerKey, blockHeight, blockTime)

					log.Warn("Producer marked as inactive and added to blacklist (cannot participate in consensus)",
						"producer", producerKey,
						"consecutiveMissedBlocks", ps.consecutiveMissedBlocks[producerKey],
						"height", blockHeight)
					needsSave = true
					ps.saveProducerToDB(producerKey)
				}
			} else {
				// Update even if not inactive yet
				needsSave = true
				ps.saveProducerToDB(producerKey)
			}
		}
	}

	ps.trySubmitRemoveBlacklistVotes(blockHeight, blockTime)

	// Also initialize tracking for new producers in the current list
	for _, producer := range currentProducers {
		producerKey := common.Bytes2Hex(producer)
		if _, exists := ps.lastParticipationTime[producerKey]; !exists {
			// New producer, initialize with 0 missed blocks
			log.Info(" is New producer, set missed block to 0", " producerKey ", producerKey)
			ps.consecutiveMissedBlocks[producerKey] = 0
			ps.isInactive[producerKey] = false
			ps.lastParticipationTime[producerKey] = blockTime
		}
	}

	// Save current block height
	if needsSave && ps.db != nil {
		key := []byte("currentBlockHeight")
		value := make([]byte, 8)
		binary.BigEndian.PutUint64(value, blockHeight)
		if err := ps.db.Put(key, value); err != nil {
			log.Error("Failed to save current block height", "error", err)
		}
	}

	// Periodically cleanup old producer data that are no longer active
	if blockHeight-ps.lastCleanupBlockHeight >= CleanupIntervalBlocks {
		log.Info("clean up old producers ", " last clean height: ", ps.lastCleanupBlockHeight, " blockHeight ", blockHeight)
		ps.cleanupOldProducers(currentProducers, blockHeight)
		ps.lastCleanupBlockHeight = blockHeight
	}
}

// cleanupOldProducers removes producer data that are no longer in the current producer list
// and haven't participated for a long time (CleanupThresholdDays)
func (ps *ProducerStats) cleanupOldProducers(currentProducers [][]byte, currentHeight uint64) {
	// Create a set of current producers for quick lookup
	producerSet := make(map[string]bool)
	for _, producer := range currentProducers {
		producerKey := common.Bytes2Hex(producer)
		producerSet[producerKey] = true
	}

	// Calculate cleanup threshold time
	cleanupThresholdTime := time.Now().AddDate(0, 0, -CleanupThresholdDays).Unix()
	cleanupThresholdHeight := currentHeight - (CleanupThresholdDays * 24 * 60 * 60 / 3) // Assuming 3 seconds per block

	// Find producers to cleanup
	producersToCleanup := make([]string, 0)
	for producerKey := range ps.lastParticipationTime {
		// Skip if producer is in current list
		if producerSet[producerKey] {
			continue
		}

		// Check if producer hasn't participated for a long time
		lastTimeSec := ps.lastParticipationTime[producerKey]
		lastHeight := ps.lastBlockHeight[producerKey]

		// Cleanup if:
		// 1. Not in current producer list AND
		// 2. Last participation was more than CleanupThresholdDays ago OR
		// 3. Last participation height is more than cleanupThresholdHeight blocks ago
		if (lastTimeSec > 0 && int64(lastTimeSec) < cleanupThresholdTime) || lastHeight < cleanupThresholdHeight {
			producersToCleanup = append(producersToCleanup, producerKey)
		}
	}

	// Remove from memory and database
	cleanedCount := 0
	for _, producerKey := range producersToCleanup {
		// Remove from memory
		delete(ps.lastParticipationTime, producerKey)
		delete(ps.participationCount, producerKey)
		delete(ps.lastBlockHeight, producerKey)
		delete(ps.consecutiveMissedBlocks, producerKey)
		delete(ps.isInactive, producerKey)
		delete(ps.removeVoteSubmitted, producerKey)

		// Remove from database
		if ps.db != nil {
			key := []byte("producer:" + producerKey)
			if err := ps.db.Delete(key); err != nil {
				log.Error("Failed to delete producer stats from database", "producer", producerKey, "error", err)
			}
		}

		cleanedCount++
	}

	if cleanedCount > 0 {
		log.Info("Cleaned up old producer statistics",
			"producerStatsCleaned", cleanedCount,
			"height", currentHeight,
			"remainingProducers", len(ps.lastParticipationTime))
	}
}

// IsInactive checks if a producer is inactive (cannot participate in consensus)
func (ps *ProducerStats) IsInactive(producerPubKey []byte) bool {
	if len(producerPubKey) == 0 {
		return false
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	producerKey := common.Bytes2Hex(producerPubKey)
	return ps.isInactive[producerKey]
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

// GetInactiveProducers returns the list of inactive producer public keys (hex)
func (ps *ProducerStats) GetInactiveProducers() []string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	inactive := make([]string, 0, len(ps.isInactive))
	for producerKey, isInactive := range ps.isInactive {
		if isInactive {
			inactive = append(inactive, producerKey)
		}
	}
	return inactive
}

// addToBlacklist adds a producer to the blacklist
func (ps *ProducerStats) addToBlacklist(producerKey string, blockHeight uint64, _ uint64) {
	// Submit blacklist vote to contract (best-effort)
	lastSealHeight := ps.lastBlockHeight[producerKey]
	if err := ps.submitBlacklistVote(producerKey, lastSealHeight); err != nil {
		log.Error("Submit blacklist vote failed",
			"producer", producerKey,
			"lastSealHeight", lastSealHeight,
			"error", err)
	}
}

func (ps *ProducerStats) submitBlacklistVote(producerKey string, lastSealBlockHeight uint64) error {
	if ps.blacklistOracle == nil {
		return nil
	}
	targetPubKey := common.Hex2Bytes(producerKey)
	if res, err := ps.blacklistOracle.IsBlacklisted(targetPubKey); res == true || err != nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("blacklisted : %s", producerKey)
	}
	return ps.blacklistOracle.SubmitBlacklistVote(producerKey, lastSealBlockHeight)
}

func (ps *ProducerStats) trySubmitRemoveBlacklistVotes(blockHeight uint64, blockTime uint64) {
	if ps.blacklistOracle == nil {
		return
	}
	now := time.Unix(int64(blockTime), 0)
	for producerKey, inactive := range ps.isInactive {
		if !inactive || ps.removeVoteSubmitted[producerKey] {
			continue
		}
		lastTimeSec := ps.lastParticipationTime[producerKey]
		if lastTimeSec == 0 {
			continue
		}
		lastTime := time.Unix(int64(lastTimeSec), 0)
		if now.Sub(lastTime) < BlacklistRemovalAfter {
			continue
		}
		lastSealHeight := ps.lastBlockHeight[producerKey]
		if lastSealHeight == 0 {
			lastSealHeight = blockHeight
		}
		if err := ps.blacklistOracle.RemoveBlacklistVote(producerKey, lastSealHeight); err != nil {
			log.Error("Submit remove blacklist vote failed",
				"producer", producerKey,
				"lastSealHeight", lastSealHeight,
				"error", err)
			continue
		}
		ps.removeVoteSubmitted[producerKey] = true
		ps.saveProducerToDB(producerKey)
	}
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

// getNowWithLock returns the current block time when the caller already holds
// the mutex (Lock or RLock). This avoids attempting to acquire the lock twice,
// which can deadlock when invoked from code paths that already hold ps.mu.
func (ps *ProducerStats) getNowWithLock() time.Time {
	now := ps.currentBlockTime
	if now.IsZero() {
		return time.Now()
	}
	return now
}

func durationSinceSec(nowSec int64, pastSec uint64) time.Duration {
	if nowSec < 0 || pastSec > uint64(nowSec) {
		return 0
	}
	return time.Duration(int64(nowSec)-int64(pastSec)) * time.Second
}

// ProducerStatsData represents the serialized data for a producer
type ProducerStatsData struct {
	LastParticipationTime   int64  `json:"lastParticipationTime"`
	ParticipationCount      uint64 `json:"participationCount"`
	LastBlockHeight         uint64 `json:"lastBlockHeight"`
	ConsecutiveMissedBlocks uint64 `json:"consecutiveMissedBlocks"`
	IsInactive              bool   `json:"isInactive"`
	RemoveVoteSubmitted     bool   `json:"removeVoteSubmitted"`
}

// saveProducerToDB saves a producer's statistics to the database
func (ps *ProducerStats) saveProducerToDB(producerKey string) {
	if ps.db == nil {
		return
	}
	fmt.Println("save ProducerToDB>>> ", producerKey)
	data := ProducerStatsData{
		LastParticipationTime:   int64(ps.lastParticipationTime[producerKey]),
		ParticipationCount:      ps.participationCount[producerKey],
		LastBlockHeight:         ps.lastBlockHeight[producerKey],
		ConsecutiveMissedBlocks: ps.consecutiveMissedBlocks[producerKey],
		IsInactive:              ps.isInactive[producerKey],
		RemoveVoteSubmitted:     ps.removeVoteSubmitted[producerKey],
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
		ps.currentBlockHeight = binary.BigEndian.Uint64(value)
		ps.lastProcessedBlockHeight = ps.currentBlockHeight
		// Initialize lastCleanupBlockHeight to current height to avoid immediate cleanup
		ps.lastCleanupBlockHeight = ps.currentBlockHeight
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
		ps.participationCount[producerKey] = data.ParticipationCount
		ps.lastBlockHeight[producerKey] = data.LastBlockHeight
		ps.consecutiveMissedBlocks[producerKey] = data.ConsecutiveMissedBlocks
		ps.isInactive[producerKey] = data.IsInactive
		ps.removeVoteSubmitted[producerKey] = data.RemoveVoteSubmitted

		count++
	}

	log.Info("Loaded producer statistics from database",
		"producerCount", count,
		"currentBlockHeight", ps.currentBlockHeight)
	return it.Error()
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
