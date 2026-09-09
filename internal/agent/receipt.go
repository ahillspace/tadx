package agent

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
)

type installationReceipt struct {
	Version  int               `json:"version"`
	Packages map[string]string `json:"packages"`
	raw      []byte
}

func receiptPath(base string) string { return path.Join(path.Dir(base), ".tadx-skill-receipt.json") }

func readReceipt(root *os.Root, base string) (installationReceipt, error) {
	r := installationReceipt{Version: 1, Packages: map[string]string{}}
	location := receiptPath(base)
	info, err := root.Lstat(location)
	if errors.Is(err, fs.ErrNotExist) {
		return r, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return r, errors.New("skill ownership receipt must be a bounded regular file")
	}
	file, err := root.Open(location)
	if err != nil {
		return r, errors.New("cannot read skill ownership receipt")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 4097))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(data) > 4096 || json.Unmarshal(data, &r) != nil || r.Version != 1 || len(r.Packages) > 2 {
		return r, errors.New("skill ownership receipt is invalid; inspect the target installation metadata")
	}
	for name, value := range r.Packages {
		digest, err := hex.DecodeString(value)
		if name != "tadx" && name != "tadx-pulse" || err != nil || len(digest) != 32 {
			return r, errors.New("skill ownership receipt contains an invalid package fingerprint")
		}
	}
	r.raw = data
	return r, nil
}

func receiptUnchanged(root *os.Root, base string, before installationReceipt) error {
	current, err := readReceipt(root, base)
	if err != nil || !bytes.Equal(before.raw, current.raw) {
		return errors.New("skill ownership receipt changed during preparation; retry after reviewing the installation")
	}
	return nil
}

// Commit the receipt only after all package moves succeed, while rollback is
// still possible. A failed write leaves the old receipt and old packages intact.
func (in Installer) writeReceipt(root *os.Root, base string, packages map[string]string) error {
	data, err := json.Marshal(installationReceipt{Version: 1, Packages: packages})
	if err != nil {
		return errors.New("cannot encode skill ownership receipt")
	}
	location := receiptPath(base)
	if err := checkParents(root, path.Dir(location)); err != nil {
		return err
	}
	stage := path.Join(path.Dir(location), ".tadx-skill-receipt-"+rand.Text()+".tmp")
	file, err := root.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.New("cannot stage skill ownership receipt")
	}
	defer root.Remove(stage)
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("cannot write skill ownership receipt")
	}
	commit := in.commitReceipt
	if commit == nil {
		commit = func(root *os.Root, from, to string) error { return root.Rename(from, to) }
	}
	if err := commit(root, stage, location); err != nil {
		return errors.New("cannot commit skill ownership receipt; previous packages restored")
	}
	return nil
}
