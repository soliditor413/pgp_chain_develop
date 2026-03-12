package pbft

// BlacklistOracle submits blacklist votes to an external system.
type BlacklistOracle interface {
	SubmitBlacklistVote(producerKey string, lastSealBlockHeight uint64) error
	RemoveBlacklistVote(producerKey string, lastSealBlockHeight uint64) error
	IsBlacklisted(dposPublicKey []byte) (bool, error)
	HasVoted(dposPublicKey []byte) (bool, error)
}
