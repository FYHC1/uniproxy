//go:build !windows

package processguard

import "testing"

func TestEnableIsPortableNoOp(t *testing.T) {
	for range 3 {
		if err := Enable(); err != nil {
			t.Fatalf("non-Windows Enable() = %v, want nil", err)
		}
	}
}
