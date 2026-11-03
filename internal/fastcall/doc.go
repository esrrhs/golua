// Package fastcall provides experimental amd64 helpers that invoke a C
// function pointer on the current Go stack without runtime.cgocall.
//
// This is intentionally unsafe: there is no entersyscall/exitsyscall, no
// g→g0 stack switch, and no GC coordination with C frames. It exists so
// golua can measure how much of the Go→C cost is the cgocall floor.
//
// Important limitation: C code that calls back into Go via cgo //export
// (or purego.NewCallback) expects a matching cgocall entry. Using Call*
// then //export typically crashes the runtime (morestack on g0). Lua C
// extensions that call lua_* therefore cannot use this path; keep the
// default cgo trampoline (or purego.SyscallN, which still uses cgocall).
package fastcall
