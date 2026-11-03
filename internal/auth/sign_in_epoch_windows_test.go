package auth

import (
	"os"
	"strings"
	"testing"
)

func TestFailedEpochReplacementPreservesPriorState(t *testing.T) {
	directory := t.TempDir()
	owner, err := acquireEpochOwner(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release()
	if err := owner.writeEpoch(1); err != nil {
		t.Fatal(err)
	}
	// A non-cooperating scanner can hold a Windows handle that denies rename.
	// This real OS failure must preserve the complete earlier state.
	scanner, err := os.Open(signInEpochPath(directory, epochTestKey))
	if err != nil {
		t.Fatal(err)
	}
	defer scanner.Close()
	if err := owner.writeEpoch(2); err == nil {
		t.Fatal("replacement unexpectedly bypassed the scanner's handle")
	}
	if err := scanner.Close(); err != nil {
		t.Fatal(err)
	}
	if epoch, err := owner.readEpoch(); err != nil || epoch != 1 {
		t.Fatalf("failed replacement changed epoch: %d, %v", epoch, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("failed replacement leaked staging: %s", entry.Name())
		}
	}
	if err := owner.writeEpoch(2); err != nil {
		t.Fatal(err)
	}
	if epoch, err := owner.readEpoch(); err != nil || epoch != 2 {
		t.Fatalf("retry did not install the next complete epoch: %d, %v", epoch, err)
	}
}
