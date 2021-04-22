// +build !darwin,!freebsd

package syscallhelper

import "syscall"

func RlimitMax(rlimit syscall.Rlimit) uint64 {
	return rlimit.Max
}
