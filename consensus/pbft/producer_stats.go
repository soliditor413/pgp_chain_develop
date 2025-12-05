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
	InactiveThreshold uint64 = 2000
	// producerStatsDBName is the database name for storing producer statistics
	producerStatsDBName = "producer_stats"
	// CleanupThresholdDays is the number of days after which inactive producers not in current list can be cleaned up
	CleanupThresholdDays = 30
	// CleanupIntervalBlocks is the number of blocks between cleanup operations
	CleanupIntervalBlocks uint64 = 10000
)

// dbInterface combines the interfaces we need for persistence
type dbInterface interface {
	ethdb.KeyValueWriter
	ethdb.KeyValueReader
	ethdb.Iteratee
}

// BlacklistEntry represents a permanent record of an inactive producer
type BlacklistEntry struct {
	ProducerPublicKey       string    `json:"producerPublicKey"`
	AddedAt                 time.Time `json:"addedAt"`                 // When the producer was added to blacklist
	AddedAtBlockHeight      uint64    `json:"addedAtBlockHeight"`      // Block height when added to blacklist
	ConsecutiveMissedBlocks uint64    `json:"consecutiveMissedBlocks"` // Consecutive missed blocks when added
	LastParticipationTime   time.Time `json:"lastParticipationTime"`   // Last participation time before being blacklisted
	LastBlockHeight         uint64    `json:"lastBlockHeight"`         // Last block height before being blacklisted
}

// ProducerStats tracks the participation statistics for each producer
type ProducerStats struct {
	mu                       sync.RWMutex
	db                       dbInterface                // Database for persistence
	dataDir                  string                     // Data directory path
	lastParticipationTime    map[string]time.Time       // key: producer public key (hex), value: last participation time
	participationCount       map[string]uint64          // key: producer public key (hex), value: participation count
	lastBlockHeight          map[string]uint64          // key: producer public key (hex), value: last block height
	consecutiveMissedBlocks  map[string]uint64          // key: producer public key (hex), value: consecutive missed blocks
	isInactive               map[string]bool            // key: producer public key (hex), value: true if inactive (cannot participate in consensus)
	blacklist                map[string]*BlacklistEntry // key: producer public key (hex), value: blacklist entry (permanent record)
	currentBlockHeight       uint64                     // current block height for tracking
	lastProcessedBlockHeight uint64                     // last processed block height to avoid duplicate processing
	lastCleanupBlockHeight   uint64                     // last block height when cleanup was performed
}

// NewProducerStats creates a new ProducerStats instance
// dataDir: the data directory for storing the database, empty string means no persistence
func NewProducerStats(dataDir string) (*ProducerStats, error) {
	ps := &ProducerStats{
		dataDir:                  dataDir,
		lastParticipationTime:    make(map[string]time.Time),
		participationCount:       make(map[string]uint64),
		lastBlockHeight:          make(map[string]uint64),
		consecutiveMissedBlocks:  make(map[string]uint64),
		isInactive:               make(map[string]bool),
		blacklist:                make(map[string]*BlacklistEntry),
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
	participationTime := time.Unix(int64(blockTime), 0)

	ps.lastParticipationTime[producerKey] = participationTime
	ps.participationCount[producerKey]++
	ps.lastBlockHeight[producerKey] = blockHeight

	// Reset consecutive missed blocks when producer participates
	ps.consecutiveMissedBlocks[producerKey] = 0

	// Remove inactive status if producer participates again
	if ps.isInactive[producerKey] {
		ps.isInactive[producerKey] = false
		log.Info("Producer removed from inactive status due to participation",
			"producer", producerKey,
			"height", blockHeight)
	}

	log.Debug("Record producer participation",
		"producer", producerKey,
		"height", blockHeight,
		"time", participationTime,
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

// UpdateBlockHeight updates the block height and checks for inactive producers
// This should be called when a new block is inserted, with the list of current producers
func (ps *ProducerStats) UpdateBlockHeight(blockHeight uint64, currentProducers [][]byte) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Avoid processing the same block height multiple times
	if blockHeight <= ps.lastProcessedBlockHeight {
		return
	}

	// Update current block height
	ps.currentBlockHeight = blockHeight
	ps.lastProcessedBlockHeight = blockHeight

	// Create a set of current producers for quick lookup
	producerSet := make(map[string]bool)
	for _, producer := range currentProducers {
		producerKey := common.Bytes2Hex(producer)
		producerSet[producerKey] = true
	}

	// Update consecutive missed blocks for all known producers
	// Only track producers that are in the current producer list
	needsSave := false
	for producerKey := range ps.lastParticipationTime {
		// Only track if this producer is in the current producer list
		if !producerSet[producerKey] {
			continue
		}

		// Check if this producer participated in the last block
		// If lastBlockHeight is less than current block height, they missed this block
		if ps.lastBlockHeight[producerKey] < blockHeight {
			ps.consecutiveMissedBlocks[producerKey]++

			// Check if should be marked as inactive
			if ps.consecutiveMissedBlocks[producerKey] >= InactiveThreshold {
				if !ps.isInactive[producerKey] {
					ps.isInactive[producerKey] = true

					// Add to blacklist (permanent record)
					ps.addToBlacklist(producerKey, blockHeight)

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

	// Also initialize tracking for new producers in the current list
	for _, producer := range currentProducers {
		producerKey := common.Bytes2Hex(producer)
		if _, exists := ps.lastParticipationTime[producerKey]; !exists {
			// New producer, initialize with 0 missed blocks
			ps.consecutiveMissedBlocks[producerKey] = 0
			ps.isInactive[producerKey] = false
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
	cleanupThresholdTime := time.Now().AddDate(0, 0, -CleanupThresholdDays)
	cleanupThresholdHeight := currentHeight - (CleanupThresholdDays * 24 * 60 * 60 / 3) // Assuming 3 seconds per block

	// Find producers to cleanup
	producersToCleanup := make([]string, 0)
	for producerKey := range ps.lastParticipationTime {
		// Skip if producer is in current list
		if producerSet[producerKey] {
			continue
		}

		// Check if producer hasn't participated for a long time
		lastTime := ps.lastParticipationTime[producerKey]
		lastHeight := ps.lastBlockHeight[producerKey]

		// Cleanup if:
		// 1. Not in current producer list AND
		// 2. Last participation was more than CleanupThresholdDays ago OR
		// 3. Last participation height is more than cleanupThresholdHeight blocks ago
		if lastTime.Before(cleanupThresholdTime) || lastHeight < cleanupThresholdHeight {
			producersToCleanup = append(producersToCleanup, producerKey)
		}
	}

	// Remove from memory and database
	cleanedCount := 0
	blacklistCleanedCount := 0
	for _, producerKey := range producersToCleanup {
		// Remove from memory
		delete(ps.lastParticipationTime, producerKey)
		delete(ps.participationCount, producerKey)
		delete(ps.lastBlockHeight, producerKey)
		delete(ps.consecutiveMissedBlocks, producerKey)
		delete(ps.isInactive, producerKey)

		// Clean up blacklist from memory (but keep in database)
		if _, exists := ps.blacklist[producerKey]; exists {
			delete(ps.blacklist, producerKey)
			blacklistCleanedCount++
		}

		// Remove from database (but keep blacklist entry in DB)
		if ps.db != nil {
			key := []byte("producer:" + producerKey)
			if err := ps.db.Delete(key); err != nil {
				log.Error("Failed to delete producer stats from database", "producer", producerKey, "error", err)
			}
			// Note: Blacklist entries in database are not deleted as they are permanent records
		}

		cleanedCount++
	}

	// Also cleanup blacklist entries from memory that are not in current producer list
	// but keep them in database
	for producerKey := range ps.blacklist {
		// Skip if producer is in current list
		if producerSet[producerKey] {
			continue
		}

		// Check if blacklist entry is old enough to be removed from memory
		entry := ps.blacklist[producerKey]
		if entry != nil {
			// Remove from memory if added more than CleanupThresholdDays ago
			if time.Since(entry.AddedAt) > time.Duration(CleanupThresholdDays)*24*time.Hour {
				delete(ps.blacklist, producerKey)
				blacklistCleanedCount++
			}
		}
	}

	if cleanedCount > 0 || blacklistCleanedCount > 0 {
		log.Info("Cleaned up old producer statistics",
			"producerStatsCleaned", cleanedCount,
			"blacklistMemoryCleaned", blacklistCleanedCount,
			"height", currentHeight,
			"remainingProducers", len(ps.lastParticipationTime),
			"remainingBlacklistInMemory", len(ps.blacklist))
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

// addToBlacklist adds a producer to the permanent blacklist
func (ps *ProducerStats) addToBlacklist(producerKey string, blockHeight uint64) {
	// Check if already in blacklist
	if _, exists := ps.blacklist[producerKey]; exists {
		return // Already in blacklist, don't update
	}

	// Create blacklist entry
	entry := &BlacklistEntry{
		ProducerPublicKey:       producerKey,
		AddedAt:                 time.Now(),
		AddedAtBlockHeight:      blockHeight,
		ConsecutiveMissedBlocks: ps.consecutiveMissedBlocks[producerKey],
		LastParticipationTime:   ps.lastParticipationTime[producerKey],
		LastBlockHeight:         ps.lastBlockHeight[producerKey],
	}

	ps.blacklist[producerKey] = entry

	// Save to database
	ps.saveBlacklistEntryToDB(producerKey, entry)
}

// IsInBlacklist checks if a producer is in the permanent blacklist
// This method checks both memory and database
func (ps *ProducerStats) IsInBlacklist(producerPubKey []byte) bool {
	if len(producerPubKey) == 0 {
		return false
	}

	producerKey := common.Bytes2Hex(producerPubKey)

	ps.mu.RLock()
	// First check memory
	_, exists := ps.blacklist[producerKey]
	ps.mu.RUnlock()

	if exists {
		return true
	}

	// If not in memory, check database
	if ps.db != nil {
		key := []byte("blacklist:" + producerKey)
		if value, err := ps.db.Get(key); err == nil && len(value) > 0 {
			// Load into memory for future queries
			var entry BlacklistEntry
			if err := json.Unmarshal(value, &entry); err == nil {
				ps.mu.Lock()
				ps.blacklist[producerKey] = &entry
				ps.mu.Unlock()
				return true
			}
		}
	}

	return false
}

// GetBlacklistEntry returns the blacklist entry for a producer
// This method checks both memory and database
func (ps *ProducerStats) GetBlacklistEntry(producerPubKey []byte) *BlacklistEntry {
	if len(producerPubKey) == 0 {
		return nil
	}

	producerKey := common.Bytes2Hex(producerPubKey)

	ps.mu.RLock()
	entry := ps.blacklist[producerKey]
	ps.mu.RUnlock()

	if entry != nil {
		// Return a copy to avoid external modification
		entryCopy := *entry
		return &entryCopy
	}

	// If not in memory, check database
	if ps.db != nil {
		key := []byte("blacklist:" + producerKey)
		if value, err := ps.db.Get(key); err == nil && len(value) > 0 {
			var dbEntry BlacklistEntry
			if err := json.Unmarshal(value, &dbEntry); err == nil {
				// Load into memory for future queries
				ps.mu.Lock()
				ps.blacklist[producerKey] = &dbEntry
				ps.mu.Unlock()
				// Return a copy
				entryCopy := dbEntry
				return &entryCopy
			}
		}
	}

	return nil
}

// GetBlacklist returns all blacklist entries
// This method loads all entries from database if not in memory
func (ps *ProducerStats) GetBlacklist() map[string]*BlacklistEntry {
	result := make(map[string]*BlacklistEntry)

	ps.mu.RLock()
	// First add entries from memory
	for key, entry := range ps.blacklist {
		entryCopy := *entry
		result[key] = &entryCopy
	}
	ps.mu.RUnlock()

	// Also load from database if available
	if ps.db != nil {
		blacklistPrefix := []byte("blacklist:")
		it := ps.db.NewIteratorWithPrefix(blacklistPrefix)
		defer it.Release()

		for it.Next() {
			key := it.Key()
			value := it.Value()

			// Extract producer key from "blacklist:xxxxx"
			if len(key) <= len(blacklistPrefix) {
				continue
			}
			producerKey := string(key[len(blacklistPrefix):])

			// Skip if already in result (from memory)
			if _, exists := result[producerKey]; exists {
				continue
			}

			var entry BlacklistEntry
			if err := json.Unmarshal(value, &entry); err == nil {
				// Add to result
				result[producerKey] = &entry
				// Also load into memory for future queries
				ps.mu.Lock()
				ps.blacklist[producerKey] = &entry
				ps.mu.Unlock()
			}
		}
	}

	return result
}

// GetBlacklistProducerKeys returns the list of producer public keys in the blacklist
// This method loads all keys from database if not in memory
func (ps *ProducerStats) GetBlacklistProducerKeys() []string {
	keysMap := make(map[string]bool)

	ps.mu.RLock()
	// First add keys from memory
	for producerKey := range ps.blacklist {
		keysMap[producerKey] = true
	}
	ps.mu.RUnlock()

	// Also load from database if available
	if ps.db != nil {
		blacklistPrefix := []byte("blacklist:")
		it := ps.db.NewIteratorWithPrefix(blacklistPrefix)
		defer it.Release()

		for it.Next() {
			key := it.Key()
			// Extract producer key from "blacklist:xxxxx"
			if len(key) > len(blacklistPrefix) {
				producerKey := string(key[len(blacklistPrefix):])
				keysMap[producerKey] = true
			}
		}
	}

	// Convert map to slice
	keys := make([]string, 0, len(keysMap))
	for producerKey := range keysMap {
		keys = append(keys, producerKey)
	}
	return keys
}

// ProducerStatsData represents the serialized data for a producer
type ProducerStatsData struct {
	LastParticipationTime   int64  `json:"lastParticipationTime"`
	ParticipationCount      uint64 `json:"participationCount"`
	LastBlockHeight         uint64 `json:"lastBlockHeight"`
	ConsecutiveMissedBlocks uint64 `json:"consecutiveMissedBlocks"`
	IsInactive              bool   `json:"isInactive"`
}

// saveProducerToDB saves a producer's statistics to the database
func (ps *ProducerStats) saveProducerToDB(producerKey string) {
	if ps.db == nil {
		return
	}

	data := ProducerStatsData{
		LastParticipationTime:   ps.lastParticipationTime[producerKey].Unix(),
		ParticipationCount:      ps.participationCount[producerKey],
		LastBlockHeight:         ps.lastBlockHeight[producerKey],
		ConsecutiveMissedBlocks: ps.consecutiveMissedBlocks[producerKey],
		IsInactive:              ps.isInactive[producerKey],
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

// saveBlacklistEntryToDB saves a blacklist entry to the database
func (ps *ProducerStats) saveBlacklistEntryToDB(producerKey string, entry *BlacklistEntry) {
	if ps.db == nil {
		return
	}

	jsonData, err := json.Marshal(entry)
	if err != nil {
		log.Error("Failed to marshal blacklist entry", "producer", producerKey, "error", err)
		return
	}

	key := []byte("blacklist:" + producerKey)
	if err := ps.db.Put(key, jsonData); err != nil {
		log.Error("Failed to save blacklist entry to database", "producer", producerKey, "error", err)
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
		ps.lastParticipationTime[producerKey] = time.Unix(data.LastParticipationTime, 0)
		ps.participationCount[producerKey] = data.ParticipationCount
		ps.lastBlockHeight[producerKey] = data.LastBlockHeight
		ps.consecutiveMissedBlocks[producerKey] = data.ConsecutiveMissedBlocks
		ps.isInactive[producerKey] = data.IsInactive

		count++
	}

	// Load blacklist entries
	blacklistPrefix := []byte("blacklist:")
	blacklistIt := ps.db.NewIteratorWithPrefix(blacklistPrefix)
	defer blacklistIt.Release()

	blacklistCount := 0
	for blacklistIt.Next() {
		key := blacklistIt.Key()
		value := blacklistIt.Value()

		// Extract producer key from "blacklist:xxxxx"
		if len(key) <= len(blacklistPrefix) {
			continue
		}
		producerKey := string(key[len(blacklistPrefix):])

		var entry BlacklistEntry
		if err := json.Unmarshal(value, &entry); err != nil {
			log.Error("Failed to unmarshal blacklist entry", "producer", producerKey, "error", err)
			continue
		}

		// Restore blacklist entry
		ps.blacklist[producerKey] = &entry
		blacklistCount++
	}

	log.Info("Loaded producer statistics from database",
		"producerCount", count,
		"blacklistCount", blacklistCount,
		"currentBlockHeight", ps.currentBlockHeight)
	return blacklistIt.Error()
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
