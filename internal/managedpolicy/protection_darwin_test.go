package managedpolicy

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDarwinNativeExtendedACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := checkExtendedACL(int(f.Fd())); err != nil {
		t.Fatalf("plain file: %v", err)
	}
	for _, tc := range []struct {
		acl   string
		valid bool
	}{
		{"everyone allow read,readattr,readextattr,readsecurity", true},
		{"everyone deny delete", true},
		{"everyone allow write", false},
		{"everyone allow writesecurity", false},
		{"everyone allow chown", false},
	} {
		if output, err := exec.Command("/bin/chmod", "-N", path).CombinedOutput(); err != nil {
			t.Fatalf("remove fixture ACL: %v %s", err, output)
		}
		if output, err := exec.Command("/bin/chmod", "+a", tc.acl, path).CombinedOutput(); err != nil {
			t.Fatalf("set fixture ACL: %v %s", err, output)
		}
		if err := checkExtendedACL(int(f.Fd())); (err == nil) != tc.valid {
			t.Fatalf("%s: valid=%v error=%v", tc.acl, tc.valid, err)
		}
	}
}
