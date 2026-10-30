package auth

import (
	"os"
	"strings"
	"sync"
	"testing"
)

// TestSignInEpochReadersNeverSeeAPartialWrite reads the epoch while another
// command advances it. Readers do not hold the PAT's lock, so every read must
// observe a complete earlier or later epoch.
func TestSignInEpochReadersNeverSeeAPartialWrite(t *testing.T) {
	directory := t.TempDir()
	const key = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := writeSignInEpoch(directory, key, 1); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				if epoch, err := readSignInEpoch(directory, key); err != nil || epoch == 0 {
					t.Errorf("readSignInEpoch() = %d, %v; want a complete epoch", epoch, err)
					return
				}
			}
		})
	}
	for epoch := uint64(2); epoch < 200; epoch++ {
		if err := writeSignInEpoch(directory, key, epoch); err != nil {
			t.Fatal(err)
		}
	}
	close(done)
	readers.Wait()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("staged epoch file left behind: %s", entry.Name())
		}
	}
}
