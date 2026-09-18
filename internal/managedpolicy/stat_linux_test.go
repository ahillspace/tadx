package managedpolicy

import "golang.org/x/sys/unix"

func setTestStat(stat *unix.Stat_t, mode uint32, links uint64) { stat.Mode = mode; stat.Nlink = links }
