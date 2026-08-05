package workspace

import "testing"

func TestHashValidatesCapabilityFormat(t *testing.T) {
	t.Parallel()
	if _, err := Hash("short"); err == nil {
		t.Fatal("Hash() accepted a short workspace key")
	}
	if got, err := Hash("abcdefghijklmnopqrstuvwxyzABCDEFGH123456789"); err != nil || len(got) != 64 {
		t.Fatalf("Hash() = %q, %v", got, err)
	}
}
