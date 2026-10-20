// Copyright (c) 2017-2019 The Elastos Foundation
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.
//

package dpos

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/elastos/Elastos.ELA/common"
	"github.com/elastos/Elastos.ELA/core/types/payload"
)

// CheckConfirm verifies that a confirm is genuinely endorsed by the producer
// set. Besides signature validity it enforces:
//  1. the proposal sponsor is a member of the given producer set;
//  2. every vote signer is a member of the given producer set;
//  3. no signer is counted twice (duplicated votes are rejected);
//  4. the number of unique valid producer votes reaches minSignCount.
//
// producers must be the authoritative producer (arbiter) set for the height
// the confirmed block belongs to. Empty or all-zero entries are ignored.
func CheckConfirm(confirm *payload.Confirm, producers [][]byte, minSignCount int) error {
	if len(producers) == 0 {
		return errors.New("[CheckConfirm] empty producer set, can not verify confirm")
	}

	producerSet := make(map[string]struct{}, len(producers))
	for _, producer := range producers {
		if len(producer) == 0 || bytes.Equal(producer, make([]byte, len(producer))) {
			continue
		}
		producerSet[common.BytesToHexString(producer)] = struct{}{}
	}
	if len(producerSet) == 0 {
		return errors.New("[CheckConfirm] producer set has no valid member")
	}

	// The sponsor of the proposal must be a producer of the set.
	if _, ok := producerSet[common.BytesToHexString(confirm.Proposal.Sponsor)]; !ok {
		str := fmt.Sprintf("[CheckConfirm] proposal sponsor %s is not in producer set",
			common.BytesToHexString(confirm.Proposal.Sponsor))
		Error(str)
		return errors.New(str)
	}

	err := CheckProposal(&confirm.Proposal)
	if err != nil {
		return err
	}

	proposalHash := confirm.Proposal.Hash()
	signedProducers := make(map[string]struct{}, len(confirm.Votes))
	for i := range confirm.Votes {
		vote := &confirm.Votes[i]
		if !vote.Accept {
			Error("confirm contains reject vote", vote.Hash().String())
			return errors.New("[CheckConfirm] confirm contains " +
				"reject vote")
		}

		if !proposalHash.IsEqual(vote.ProposalHash) {
			Error("proposal:", proposalHash.String(), "is not equal vote proposal:", vote.ProposalHash.String())
			return errors.New("[CheckConfirm] confirm contains " +
				"invalid vote")
		}

		// One canonical key for this signer. The very same key MUST be used
		// for the membership lookup, the duplication lookup and the insert
		// below: mixing encodings (hex vs raw bytes) makes the duplication
		// lookup miss forever, which lets a single signer stuff the ballot
		// and reach minSignCount alone.
		signerKey := common.BytesToHexString(vote.Signer)

		// Only votes from producers of the set count towards finality.
		if _, ok := producerSet[signerKey]; !ok {
			str := fmt.Sprintf("[CheckConfirm] vote signer %s is not in producer set",
				signerKey)
			Error(str)
			return errors.New(str)
		}

		// Reject duplicated votes from the same signer.
		if _, ok := signedProducers[signerKey]; ok {
			str := fmt.Sprintf("[CheckConfirm] duplicated vote from signer %s",
				signerKey)
			Error(str)
			return errors.New(str)
		}

		if err := CheckVote(vote); err != nil {
			Error("check vote error", "error", err.Error())
			return errors.New("[CheckConfirm] confirm contain invalid " +
				"vote: " + err.Error())
		}

		signedProducers[signerKey] = struct{}{}
	}

	// The threshold is compared against the size of the deduplicated signer
	// set, never against a separately maintained counter: a second counter
	// can silently drift from the map and reintroduce ballot stuffing.
	uniqueVotes := len(signedProducers)
	if uniqueVotes < minSignCount {
		str := fmt.Sprintf("[CheckConfirm] error, need %d votes, got %d unique producer votes",
			minSignCount, uniqueVotes)
		Error(str)
		return errors.New(str)
	}

	return nil
}
