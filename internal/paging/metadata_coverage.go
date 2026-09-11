package paging

import "fmt"

// MetadataCoverage receives the cumulative distinct identities, not rendered matches or raw
// rows. A caller stopping at a requested limit must not claim terminal coverage.
type MetadataCoverage struct {
	total    int
	observed bool
}

func (c *MetadataCoverage) Page(total, distinct int, terminal bool) error {
	if total < 0 || distinct > total {
		return fmt.Errorf("Metadata API returned inconsistent cumulative coverage")
	}
	if c.observed && c.total != total {
		return fmt.Errorf("Metadata API collection count changed during pagination")
	}
	c.total, c.observed = total, true
	if terminal && distinct != total {
		return fmt.Errorf("Metadata API terminal coverage incomplete: observed %d of %d identities", distinct, total)
	}
	return nil
}
