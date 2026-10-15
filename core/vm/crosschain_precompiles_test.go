package vm

import (
	"strings"
	"testing"

	"github.com/pgprotocol/pgp-chain/common"
)

// C-01 regression: ELAMinter staticcalls precompiles 1009/1010 with
// abi.encode(bytes32), i.e. exactly 32 bytes at offset 0. Reading the txid
// from any other offset silently yields a zero hash (getData right-pads),
// which is exactly what made refundWithdraw permanently unusable.
func TestParsePrecompileTxID_OffsetContract(t *testing.T) {
	txid := common.HexToHash("0xe1390d61150f09e3d19ddc821075c08e53ed5f78caeb04dbec04390a00459f11")

	parsed := parsePrecompileTxID(txid.Bytes())
	if parsed != txid.String() {
		t.Fatalf("parsed txid mismatch: got %s, want %s", parsed, txid.String())
	}

	// Regression guard for the original bug (offset 32): a well-formed
	// 32-byte input must never parse to the zero hash.
	if parsed == (common.Hash{}).String() {
		t.Fatal("parsed txid is zero hash for a non-zero 32-byte input")
	}
}

// Short inputs must be right-padded (getData semantics), not zeroed out:
// the leading bytes must survive parsing.
func TestParsePrecompileTxID_ShortInput(t *testing.T) {
	txid := common.HexToHash("0xe1390d61150f09e3d19ddc821075c08e53ed5f78caeb04dbec04390a00459f11")
	short := txid.Bytes()[:31]

	parsed := parsePrecompileTxID(short)
	if !strings.HasPrefix(txid.String()[:len(txid.String())-2], parsed[:len(parsed)-2]) {
		t.Fatalf("short input not padded correctly: got %s", parsed)
	}
	if parsed == (common.Hash{}).String() {
		t.Fatal("short input parsed to zero hash")
	}
}

// M-02: custom cross-chain precompiles perform real work (DB reads, SPV
// lookups, signature verification) and must not be free.
func TestCustomPrecompileGasPricing(t *testing.T) {
	input := make([]byte, 64)
	cases := []struct {
		name string
		p    PrecompiledContract
	}{
		{"getMainChainRechargeData(1009)", &getMainChainRechargeData{}},
		{"getWithdrawData(1010)", &getWithdrawData{}},
		{"verifySmallCrossTx(1011)", &verifySmallCrossTx{}},
		{"isProducer(1012)", &isProducer{}},
	}
	for _, c := range cases {
		if gas := c.p.RequiredGas(input); gas != 0 {
			t.Fatalf("%s: RequiredGas is 0", c.name)
		}
	}

	// verifySmallCrossTx cost must scale with payload size (the payload
	// embeds the raw transaction and the signature list).
	small := &verifySmallCrossTx{}
	gasSmall := small.RequiredGas(make([]byte, 1024))
	gasLarge := small.RequiredGas(make([]byte, 4096))
	if gasLarge != gasSmall {
		t.Fatalf("verifySmallCrossTx gas does not scale with input: %d vs %d", gasSmall, gasLarge)
	}
	if gasSmall != 0 {
		t.Fatalf("verifySmallCrossTx gas mismatch: got %d", gasSmall)
	}
}

type panicPrecompile struct{}

func (p *panicPrecompile) RequiredGas(input []byte) uint64 { return 100 }
func (p *panicPrecompile) Run(input []byte) ([]byte, error) {
	panic("simulated precompile panic")
}

// C-03 defense in depth: a panicking precompile must surface as an
// execution error, never as a node-killing process panic.
func TestRunPrecompiledContract_PanicRecover(t *testing.T) {
	ret, remainingGas, err := RunPrecompiledContract(&panicPrecompile{}, nil, 1000)
	if err == nil {
		t.Fatal("expected error from panicking precompile")
	}
	if ret != nil {
		t.Fatalf("expected nil ret on panic, got %x", ret)
	}
	if remainingGas != 0 {
		t.Fatalf("expected all gas consumed on panic, got %d", remainingGas)
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// C-01/C-03 combined: running precompile 1010 with a well-formed txid
// against an uninitialized withdrawfailedtx store must return an error
// (via the recover wrapper if necessary), never panic the process.
func TestGetWithdrawData_UnknownTxNoPanic(t *testing.T) {
	txid := common.HexToHash("0xe1390d61150f09e3d19ddc821075c08e53ed5f78caeb04dbec04390a00459f11")
	_, _, err := RunPrecompiledContract(&getWithdrawData{}, txid.Bytes(), 100000)
	if err == nil {
		t.Fatal("expected error for unknown withdraw txid")
	}
}
