//go:build unix

package core

import (
	"os"
	"syscall"
)

func officialProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// officialKillTree kills the process group of p (negative pid) then the
// process itself. CommandContext alone only signals the parent, leaving
// main.lua's background `lua -e ... &` children unreaped under the suite
// timeout; a group kill matches how a Ctrl-C to the foreground job would
// tear the tree down.
func officialKillTree(p *os.Process) {
	if p == nil {
		return
	}
	_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
	_ = p.Kill()
}
