package pbft

// BlacklistOracle submits blacklist votes to an external system.
type BlacklistOracle interface {
	SubmitBlacklistVote(producerKey string, lastSealBlockHeight uint64) error
	RemoveBlacklistVote(producerKey string) error
	IsBlacklisted(dposPublicKey []byte) (bool, error)
	IsExpired(dposPublicKey []byte) (bool, error)
	HasAddVoted(dposPublicKey []byte) (bool, error)
	HasRemoveVoted(dposPublicKey []byte) (bool, error)
	StartListener(onConfirmed func([]byte), onRemoved func([]byte), getScannedHeight func() uint64, setScannedHeight func(uint64)) error
	StopListener()
}
