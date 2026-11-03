// Package fastcall is golua's Linux amd64 FFI bridge for calling C function
// pointers without going through cgo-generated stubs.
//
// Call0/Call1/Call2 enter C the way the runtime expects for //export
// re-entry: runtime.cgocall → asmcgocall (g→g0) with entersyscall/exitsyscall
// and ncgo accounting. A thin assembly trampoline invokes the function
// pointer under the SysV ABI. Argument blocks are heap-pooled so a stack
// move during a callback cannot invalidate the pointer the trampoline
// holds (stack + _cgo_topofstack was attempted; see comments in
// fastcall.go). Nested Go→C→Go(//export)→C therefore works with the
// existing cgo //export machinery.
//
// The floor is still essentially cgocall; we only skip the cgo stub /
// cgoUse / an extra C wrapper. Empty-call microbench is typically a few
// ns above stock cgo and well below ebitengine/purego.SyscallN.
//
// RawCall0/RawCall1/RawCall2 are the old unsafe path: a naked CALL on the
// current Go stack with no cgocall. They are ~10–30× faster for empty
// calls but incompatible with //export (runtime: morestack on g0 /
// exitsyscall). Keep them for microbenchmarks only.
//
// Build / linkname risks:
//   - Call* are linux/amd64 only (build-tagged); other GOOS/GOARCH panic.
//   - We //go:linkname runtime.cgocall (same as ebitengine/purego; see
//     go.dev/issue/67401). A future Go that removes that linkname breaks us.
//   - Enabled for golua via -tags golua_fastc; default production stays on
//     the stock cgo trampoline unless evidence justifies flipping it.
package fastcall
