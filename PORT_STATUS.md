# Lua 5.3.4 port status (branch `5.3.4`)

## Alignment goal
Line-by-line Go translation of PUC-Lua **5.3.4** C (`/workspace/lua-sources/lua-5.3.4/src`).
**Intentional sole exception:** Go GC replaces `lgc` (finalizers/weak tables approximated).

## Done (matches 5.3.4 C)
- Version: 5.3.4 / 503 / chunk `0x53`, package path `5.3`, copyright 1994–2017
- ISA (47 ops) + RK encoding; dump/undump interchange with PUC `luac`
- No `warn`, no `coroutine.close`, no `<const>`/`<close>`; `bit32` present
- File MT: `__index = mt`, methods on mt, no `__close`
- `lua_resume(L, from, nargs)` 5.3 signature; OpenLibs …utf8, debug, bit32
- CGO: no always-on 5.4 exports (warn/toclose/userdatauv/closeslot/…)
- Negative opcode stubs / `compat54_stubs` removed; TBC runtime removed
- `string.format`: `%+u` ignores signed flags like glibc; no 5.4 checkformat
- Differential tests refreshed for 5.3.4 (format order, coverlib string-arith floats, rkname RKASK)
- **VM leaf fastpaths** rewritten for 5.3 RK (`lvm_fastcall.go`); wired on `OP_CALL`
- Official suite (Linux, `LUA_REF` → lua-5.3.4): **25 identical** (1 vacuous), **1 known-diff (gc)**, **2 not-run (all, main)**

## Remaining / known gaps
1. **Go GC** (intentional): no pace/debt/ephemerons; `gc.lua` known-diff.
2. **VM for/tail fastpaths** still stubbed (`lvm_fastfor.go`, `lvm_fasttail.go`) — optional perf only; slow path matches PUC (incl. NaN step → 0).
3. **main/all.lua** not automated (SIGINT harness).
4. Darwin `*.darwin.expected` may be stale (re-capture with PUC 5.3.4 on macOS).
5. Thin internals: `luaD_closeprotected` (upvalue-close wrapper), warnf plumbing for `__gc` warnings / tests, `ResetThread` Go helper (not in 5.3 C API).
6. Further per-file polish still useful on larger Go ports (`lstate`, `liolib`, `lstrlib`, `ldblib`) vs C size ratios.

## Paths
- C ref: `/workspace/lua-sources/lua-5.3.4`
- Official tests: `/workspace/lua-sources/lua-5.3.4-tests` → `internal/core/testdata/official/`
- Branch: https://github.com/esrrhs/golua/tree/5.3.4
