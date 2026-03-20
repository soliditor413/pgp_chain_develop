package pbft

// BlacklistOracle submits blacklist votes to an external system.
type BlacklistOracle interface {
	SubmitBlacklistVote(producerKey string, lastSealBlockHeight uint64) error
	SubmitBlacklistVotesBatch(producerKeys []string, lastSealHeights []uint64) error
	SubmitRemoveBlacklistVotesBatch(producerKeys []string) error
	IsBlacklisted(dposPublicKey []byte) (bool, error)
	IsExpired(dposPublicKey []byte) (bool, error)
	HasAddVoted(dposPublicKey []byte) (bool, error)
	HasRemoveVoted(dposPublicKey []byte) (bool, error)
}
