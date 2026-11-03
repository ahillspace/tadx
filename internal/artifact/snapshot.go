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
	"strings"
)

func artifactTreeFingerprint(ctx context.Context, root string) (string, error) {
	tree, _, err := artifactSnapshotFingerprints(ctx, root, "")
	return tree, err
}

// artifactSnapshotFingerprints feeds the canonical file into both digests in
// the same read. The tree digest retains its existing byte-for-byte format.
func artifactSnapshotFingerprints(ctx context.Context, root, canonical string) (string, string, error) {
	digest := sha256.New()
	canonicalDigest := sha256.New()
	canonicalFound := false
	var canonicalInfo os.FileInfo
	var canonicalErr error
	if canonical != "" {
		canonicalInfo, canonicalErr = canonicalPayloadInfo(canonical)
		if canonicalErr != nil && !errors.Is(canonicalErr, os.ErrNotExist) {
			return "", "", canonicalErr
		}
	}
	err := walkArtifactTree(ctx, root, func(path string, entry os.DirEntry, name string) error {
		if err := binary.Write(digest, binary.BigEndian, uint64(len(name))); err != nil {
			return err
		}
		_, _ = io.WriteString(digest, name)
		if entry.IsDir() {
			_, _ = digest.Write([]byte{'d'})
			return nil
		}
		_, _ = digest.Write([]byte{'f'})
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return err
		}
		if err := binary.Write(digest, binary.BigEndian, uint64(info.Size())); err != nil {
			_ = file.Close()
			return err
		}
		if err := binary.Write(digest, binary.BigEndian, uint32(info.Mode().Perm())); err != nil {
			_ = file.Close()
			return err
		}
		var writer io.Writer = digest
		if !canonicalFound && canonicalInfo != nil && os.SameFile(info, canonicalInfo) {
			canonicalFound = true
			writer = io.MultiWriter(digest, canonicalDigest)
		}
		copied, copyErr := io.Copy(writer, &contextReader{ctx: ctx, reader: file})
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if copied != info.Size() {
			return errors.New("artifact file changed while its snapshot was read")
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}
	tree := "sha256:" + hex.EncodeToString(digest.Sum(nil))
	if canonicalErr != nil {
		return tree, "", canonicalErr
	}
	if canonical != "" && !canonicalFound {
		return "", "", errors.New("canonical artifact payload changed while its snapshot was read")
	}
	current := ""
	if canonicalFound {
		current = "sha256:" + hex.EncodeToString(canonicalDigest.Sum(nil))
	}
	return tree, current, nil
}

func canonicalPayloadInfo(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("canonical artifact payload must not be a symbolic link")
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("canonical artifact payload must be a regular file")
	}
	return info, nil
}

func walkArtifactTree(ctx context.Context, root string, visit func(string, os.DirEntry, string) error) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("artifact snapshot root must be a real directory, not a symbolic link")
	}
	// WalkDir's lexical depth-first order is deterministic, so each entry can
	// be hashed immediately without retaining a complete tree index.
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
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
		relative, err := filepath.Rel(root, path)
		if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("artifact snapshot path escapes its root")
		}
		if visit != nil {
			return visit(path, entry, filepath.ToSlash(relative))
		}
		return nil
	})
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
