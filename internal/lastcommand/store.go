// Package lastcommand saves one bounded result, not execution instructions.
package lastcommand

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/lock"
	"github.com/ahillspace/tadx/internal/value"
	"io"
	"os"
	"path/filepath"
)

const MaxBytes = 4 << 20

type Store struct{ Path string }

func (s Store) Read(ctx context.Context) (value.SavedExecution, error) {
	var out value.SavedExecution
	if err := ctx.Err(); err != nil {
		return out, err
	}
	info, err := os.Lstat(s.Path)
	if err != nil {
		return out, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxBytes {
		return out, errors.New("saved result is not a bounded regular file")
	}
	f, err := os.Open(s.Path)
	if err != nil {
		return out, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return out, err
	}
	if len(data) > MaxBytes {
		return out, errors.New("saved result exceeds the size limit")
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	if out.RecordedAt.IsZero() || out.Operation == "" {
		return out, errors.New("saved result is incomplete")
	}
	return out, nil
}
func (s Store) Save(ctx context.Context, out value.SavedExecution) error {
	if len(out.Result) > MaxBytes/2 {
		out.Result = nil
		out.Unavailable = "The previous result exceeded the saved-output size limit."
	}
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	if len(data) > MaxBytes {
		return errors.New("saved result exceeds the size limit")
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	h, err := lock.AcquireContext(ctx, s.Path+".lock")
	if err != nil {
		return err
	}
	defer h.Release()
	if info, err := os.Lstat(s.Path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("saved result target is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(dir, ".last-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, s.Path)
}
