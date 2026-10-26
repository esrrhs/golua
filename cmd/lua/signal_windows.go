//go:build windows

package main

// Windows stand-alone has no POSIX SIGINT / laction equivalent wired yet.
// Ctrl-C in a console is delivered differently; official main.lua is a
// Unix-only harness (CI official job is ubuntu/macos).

func enableSIGINT()  {}
func disableSIGINT() {}
