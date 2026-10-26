//go:build windows

package core

import (
	"os"
	"syscall"
)

func officialProcAttr() *syscall.SysProcAttr { return nil }

func officialKillTree(p *os.Process) {
	if p != nil {
		_ = p.Kill()
	}
}
