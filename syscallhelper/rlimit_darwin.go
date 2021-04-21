package syscallhelper

import "syscall"

func RlimitMax(_ syscall.Rlimit) uint64 {
	// https://github.com/golang/go/issues/30401
	return 24576
}
