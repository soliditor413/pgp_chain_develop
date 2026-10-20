// Copyright (c) 2017-2019 The Elastos Foundation
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.
//

package dpos

import (
	"testing"

	"github.com/elastos/Elastos.ELA/account"
	"github.com/elastos/Elastos.ELA/common"
	"github.com/elastos/Elastos.ELA/core/types/payload"
	daccount "github.com/elastos/Elastos.ELA/dpos/account"
)

func newConfirmTestAccount(t *testing.T) daccount.Account {
	t.Helper()

	ac, err := account.NewAccount()
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	return daccount.New(ac)
}

// newConfirmTestAccounts builds n distinct producer accounts.
func newConfirmTestAccounts(t *testing.T, n int) []daccount.Account {
	t.Helper()

	accts := make([]daccount.Account, 0, n)
	for i := 0; i < n; i++ {
		accts = append(accts, newConfirmTestAccount(t))
	}
	return accts
}

func producerList(accts []daccount.Account) [][]byte {
	producers := make([][]byte, len(accts))
	for i, ac := range accts {
		producers[i] = ac.PublicKeyBytes()
	}
	return producers
}

func newConfirmTestProposal(t *testing.T, sponsor daccount.Account) *payload.DPOSProposal {
	t.Helper()

	proposal, err := StartProposal(sponsor, common.Uint256{1}, 0)
	if err != nil {
		t.Fatalf("StartProposal: %v", err)
	}
	return proposal
}

func newConfirmTestVote(t *testing.T, proposal *payload.DPOSProposal, ac daccount.Account) *payload.DPOSProposalVote {
	t.Helper()

	hash := proposal.Hash()
	vote, err := StartVote(&hash, true, ac)
	if err != nil {
		t.Fatalf("StartVote: %v", err)
	}
	return vote
}

// TestCheckConfirmRejectsBallotStuffing is the regression test for the
// duplicate-vote guard. A single producer must never satisfy the threshold by
// repeating its own vote: the guard used to look up one map key and insert
// another (hex vs raw bytes), so it never fired.
func TestCheckConfirmRejectsBallotStuffing(t *testing.T) {
	const producerCount = 12
	minSignCount := MajorityCount(producerCount) // 8

	accts := newConfirmTestAccounts(t, producerCount)
	producers := producerList(accts)
	proposal := newConfirmTestProposal(t, accts[0])
	vote := newConfirmTestVote(t, proposal, accts[0])

	// Control: one genuine vote is not a majority.
	one := &payload.Confirm{
		Proposal: *proposal,
		Votes:    []payload.DPOSProposalVote{*vote},
	}
	if err := CheckConfirm(one, producers, minSignCount); err == nil {
		t.Fatalf("single vote accepted, threshold of %d not enforced", minSignCount)
	}

	// Attack: the same vote repeated minSignCount times.
	stuffed := &payload.Confirm{Proposal: *proposal}
	for i := 0; i < minSignCount; i++ {
		stuffed.Votes = append(stuffed.Votes, *vote)
	}
	if err := CheckConfirm(stuffed, producers, minSignCount); err == nil {
		t.Fatalf("ballot stuffing accepted: %d copies of one vote satisfied minSignCount=%d",
			len(stuffed.Votes), minSignCount)
	}
}

// TestCheckConfirmAcceptsDistinctProducers makes sure the guard does not reject
// legitimate confirms carrying one vote per producer.
func TestCheckConfirmAcceptsDistinctProducers(t *testing.T) {
	const producerCount = 12
	minSignCount := MajorityCount(producerCount) // 8

	accts := newConfirmTestAccounts(t, producerCount)
	producers := producerList(accts)
	proposal := newConfirmTestProposal(t, accts[0])

	confirm := &payload.Confirm{Proposal: *proposal}
	for i := 0; i < minSignCount; i++ {
		confirm.Votes = append(confirm.Votes, *newConfirmTestVote(t, proposal, accts[i]))
	}

	if err := CheckConfirm(confirm, producers, minSignCount); err != nil {
		t.Fatalf("legit confirm with %d distinct producer votes rejected: %v",
			len(confirm.Votes), err)
	}
}

// TestCheckConfirmRejectsNonProducer pins the membership check.
func TestCheckConfirmRejectsNonProducer(t *testing.T) {
	accts := newConfirmTestAccounts(t, 12)
	producers := producerList(accts)
	proposal := newConfirmTestProposal(t, accts[0])

	outsider := newConfirmTestAccount(t)
	confirm := &payload.Confirm{Proposal: *proposal}
	confirm.Votes = append(confirm.Votes, *newConfirmTestVote(t, proposal, outsider))

	if err := CheckConfirm(confirm, producers, 1); err == nil {
		t.Fatal("vote from a non-producer was counted")
	}
}

// TestMajorityCount pins the threshold formula: ceil(2n/3), never below one.
// Truncating integer division used to yield 0 for n=1 (a confirm with no votes
// passed) and undershoot two thirds for most set sizes.
func TestMajorityCount(t *testing.T) {
	cases := []struct{ n, want int }{
		{0, 0}, {1, 1}, {2, 2}, {3, 2}, {4, 3}, {5, 4},
		{6, 4}, {7, 5}, {8, 6}, {9, 6}, {10, 7}, {12, 8}, {36, 24},
	}
	for _, c := range cases {
		if got := MajorityCount(c.n); got != c.want {
			t.Errorf("MajorityCount(%d) = %d, want %d", c.n, got, c.want)
		}
	}
}
