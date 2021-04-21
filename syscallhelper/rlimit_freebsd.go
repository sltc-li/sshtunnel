// +build !darwin

package main

import "syscall"

func RlimitMax(rlimit syscall.Rlimit) int64 {
	return rlimit.Max
}
