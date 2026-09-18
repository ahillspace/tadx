package managedpolicy

import "golang.org/x/sys/unix"

func setTestStat(stat *unix.Stat_t, mode uint32, links uint64) {
	stat.Mode = uint16(mode)
	stat.Nlink = uint16(links)
}
