package pbft

// BlacklistOracle submits blacklist votes to an external system.
type BlacklistOracle interface {
	SubmitBlacklistVote(producerKey string, lastSealBlockHeight uint64) error
	// SubmitBlacklistVotesBatch 批量提交添加黑名单投票，内部使用 nonce, nonce+1, nonce+2... 签名，避免多笔同 nonce 导致 Invalid signature
	SubmitBlacklistVotesBatch(producerKeys []string, lastSealHeights []uint64) error
	// SubmitRemoveBlacklistVotesBatch 批量提交移除黑名单投票，内部使用 nonce, nonce+1, nonce+2... 签名，避免多笔同 nonce 导致 Invalid signature
	SubmitRemoveBlacklistVotesBatch(producerKeys []string) error
	IsBlacklisted(dposPublicKey []byte) (bool, error)
	IsExpired(dposPublicKey []byte) (bool, error)
	HasAddVoted(dposPublicKey []byte) (bool, error)
	HasRemoveVoted(dposPublicKey []byte) (bool, error)
	StartListener(onConfirmed func([]byte), onRemoved func([]byte), getScannedHeight func() uint64, setScannedHeight func(uint64)) error
	StopListener()
}
