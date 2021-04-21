// +build !darwin,!freebsd

package main

import "syscall"

func RlimitMax(rlimit syscall.Rlimit) uint64 {
	return rlimit.Max
}
