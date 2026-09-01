package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxArtifactSnapshotEntries = 10000

func artifactTreeFingerprint(ctx context.Context, root string) (string, error) {
	type snapshotEntry struct {
		path      string
		directory bool
	}
	var entries []snapshotEntry
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact entry %q must not be a symbolic link", entry.Name())
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("artifact entry %q must be a regular file", entry.Name())
		}
		if len(entries) >= maxArtifactSnapshotEntries {
			return errors.New("artifact tree exceeds its entry limit")
		}
		entries = append(entries, snapshotEntry{path: path, directory: entry.IsDir()})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	digest := sha256.New()
	for _, entry := range entries {
		relative, err := filepath.Rel(root, entry.path)
		if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", errors.New("artifact snapshot path escapes its root")
		}
		name := filepath.ToSlash(relative)
		if err := binary.Write(digest, binary.BigEndian, uint64(len(name))); err != nil {
			return "", err
		}
		_, _ = io.WriteString(digest, name)
		if entry.directory {
			_, _ = digest.Write([]byte{'d'})
			continue
		}
		_, _ = digest.Write([]byte{'f'})
		file, err := os.Open(entry.path)
		if err != nil {
			return "", err
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return "", err
		}
		if err := binary.Write(digest, binary.BigEndian, uint64(info.Size())); err != nil {
			_ = file.Close()
			return "", err
		}
		if err := binary.Write(digest, binary.BigEndian, uint32(info.Mode().Perm())); err != nil {
			_ = file.Close()
			return "", err
		}
		copied, copyErr := io.Copy(digest, &snapshotContextReader{ctx: ctx, reader: file})
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if copied != info.Size() {
			return "", errors.New("artifact file changed while its snapshot was read")
		}
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

type snapshotContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func sameArtifactSnapshot(left, right Item) bool {
	return left.Kind == right.Kind &&
		left.LUID == right.LUID &&
		left.Name == right.Name &&
		left.State == right.State &&
		left.ServerOrigin == right.ServerOrigin &&
		left.SiteLUID == right.SiteLUID &&
		left.BaselineFingerprint == right.BaselineFingerprint &&
		left.CurrentFingerprint == right.CurrentFingerprint &&
		left.TreeFingerprint != "" &&
		left.TreeFingerprint == right.TreeFingerprint
}

func (r *snapshotContextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
