package managedpolicy

import (
	"encoding/binary"
	"testing"
)

func TestDarwinACLBuffer(t *testing.T) {
	for _, tc := range []struct {
		name          string
		flags, rights uint32
		valid         bool
	}{
		{"read", 1, 1<<1 | 1<<7 | 1<<9 | 1<<11, true},
		{"write", 1, 1 << 2, false},
		{"delete-child", 1, 1 << 6, false},
		{"owner", 1, 1 << 13, false},
		{"security", 1, 1 << 12, false},
		{"deny", 2, 0xffffffff, true},
		{"inherit-only", 1 | 1<<8, 0xffffffff, true},
		{"unknown-kind", 3, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buffer := make([]byte, 80)
			binary.LittleEndian.PutUint32(buffer, 80)
			binary.LittleEndian.PutUint32(buffer[4:], 8)
			binary.LittleEndian.PutUint32(buffer[8:], 68)
			binary.LittleEndian.PutUint32(buffer[12:], 0x012cc16d)
			binary.LittleEndian.PutUint32(buffer[48:], 1)
			binary.LittleEndian.PutUint32(buffer[72:], tc.flags)
			binary.LittleEndian.PutUint32(buffer[76:], tc.rights)
			if err := checkDarwinACLBuffer(buffer); (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			for length := range len(buffer) {
				if err := checkDarwinACLBuffer(buffer[:length]); err == nil {
					t.Fatalf("accepted truncated buffer %d", length)
				}
			}
		})
	}
}
