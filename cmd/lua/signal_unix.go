//go:build unix

package main

import (
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
)

// SIGINT handling mirrors lua.c's docall / laction / lstop as closely as
// Go allows:
//
//   - first INT installs the lstop hook (SetInterruptHook) so a Lua busy
//     loop aborts with "interrupted!"
//   - a second INT terminates the process, matching PUC's
//     signal(SIGINT, SIG_DFL) after the first — needed when Lua is stuck
//     inside an unbreakable Go call (e.g. catastrophic string.find)
//
// signal.Reset(os.Interrupt) does not restore a kernel SIG_DFL that
// actually kills the process under the Go runtime (verified on this
// host), so the second INT calls os.Exit instead. main.lua only checks
// that kill(1) eventually fails; the exit status is not part of the
// golden.

var (
	sigOnce  sync.Once
	sigCh    chan os.Signal
	sigCount atomic.Int32
)

func ensureSIGINTGoroutine() {
	sigOnce.Do(func() {
		sigCh = make(chan os.Signal, 8)
		go func() {
			for range sigCh {
				if sigCount.Add(1) == 1 {
					if L != nil {
						L.SetInterruptHook()
					}
				} else {
					os.Exit(2) /* second INT: die like SIG_DFL */
				}
			}
		}()
	})
}

func enableSIGINT() {
	ensureSIGINTGoroutine()
	sigCount.Store(0)
	signal.Notify(sigCh, os.Interrupt)
}

func disableSIGINT() {
	if sigCh == nil {
		return
	}
	signal.Stop(sigCh)
	/* drain so a late signal does not count against the next docall */
	for {
		select {
		case <-sigCh:
		default:
			sigCount.Store(0)
			return
		}
	}
}
