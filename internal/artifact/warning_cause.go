package artifact

import (
	"errors"
	"io/fs"
	"os"
)

// warningCause renders err for a local warning without filesystem paths,
// because operating system errors carry absolute paths and warnings name
// entries by their artifact-relative names. Path and link errors keep their
// operation and cause; any other error, which can embed paths in free text,
// renders as fallback.
func warningCause(err error, fallback string) string {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Op + ": " + pathErr.Err.Error()
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return linkErr.Op + ": " + linkErr.Err.Error()
	}
	return fallback
}
