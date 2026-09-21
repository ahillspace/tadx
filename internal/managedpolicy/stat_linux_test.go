package managedpolicy

import "golang.org/x/sys/unix"

func setTestStat(stat *unix.Stat_t, mode uint32, links uint64) {
	stat.Mode = mode
	setTestLinkCount(&stat.Nlink, links)
}

// Linux uses different link-count widths on amd64 and arm64.
func setTestLinkCount[T ~uint32 | ~uint64](target *T, links uint64) {
	*target = T(links)
}
