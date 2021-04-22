// +build !darwin

package syscallhelper

import "syscall"

func RlimitMax(rlimit syscall.Rlimit) int64 {
	return rlimit.Max
}
