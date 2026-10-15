package spv

import (
	"strings"
	"testing"
)

// C-03 regression: TrySetRechargeDataFromSpvService is reachable from the
// public precompile getMainChainRechargeData (address 1009). With the SPV
// service not initialized it must return a plain error instead of
// dereferencing nil and crashing the node (observed SIGSEGV).
func TestTrySetRechargeDataFromSpvService_NilService(t *testing.T) {
	if SpvService != nil {
		t.Skip("SpvService already initialized in this test environment")
	}
	// A hash absent from the (uninitialized) local db takes the SPV fetch
	// path, which is where the nil dereference lived.
	err := TrySetRechargeDataFromSpvService("0xe1390d61150f09e3d19ddc821075c08e53ed5f78caeb04dbec04390a00459f11")
	if err == nil {
		t.Fatal("expected error when SpvService is nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Short hashes must be rejected before slicing elaHash[0:2].
func TestTrySetRechargeDataFromSpvService_ShortHash(t *testing.T) {
	for _, h := range []string{"", "0"} {
		if err := TrySetRechargeDataFromSpvService(h); err == nil {
			t.Fatalf("expected error for short hash %q", h)
		}
	}
}
