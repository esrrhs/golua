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
- CGO: no always-on 5.4 exports (warn/toclose/userdatauv/closeslot/setcstacklimit/…)
- Negative opcode stubs / `compat54_stubs` removed; TBC runtime removed
- `string.format`: `%+u` ignores signed flags like glibc; no 5.4 checkformat
- `cmd/lua`: options match `lua.c` 5.3.4 (no `-W`, no `-l g=mod`)
- `debug` lib: no `setcstacklimit`; `getinfo` default `flnStu` (no `r`); `gethook` always 3 results
- Arg checks: `rawlen` / `setmetatable` / `gsub` use fixed `luaL_argcheck` messages (no `, got TYPE`)
- `collectgarbage`: 5.3 option list; `setstepmul` clamps `<40`→40; default `LUAI_GCMUL=200`
- `luaD_closeprotected` removed; call sites use `luaF_close` like `ldo.c`
- Dead 5.4 `ltm` helpers and unused for/tail fastpath stubs deleted
- VM leaf fastpaths rewritten for 5.3 RK (`lvm_fastcall.go`); wired on `OP_CALL`
- Table: `hashint` = `hashpow2` (5.3.4, not 5.4 `hashmod`); `luaH_getn` = 5.3.4 binary/unbound search (no alimit shrink)
- `OP_TAILCALL` to C falls through to following `RETURN` like `lvm.c` 5.3.4 (count hooks)
- `utf8.charpattern` = `[\0-\x7F\xC2-\xF4][\x80-\xBF]*`; surrogates allowed
- stdlibs: `table.remove` arg#1; `os.time` field order/L_MAXDATEFIELD; `io.lines` returns 1; unpack `z`; `require` returns 1; `io.popen` modes `r`/`w`/`re`/`we` else EINVAL; file userdata 1 uservalue; 5.3 get/setuservalue
- Differential Linux: **83/83 identical** vs PUC `lua-5.3.4/src/lua`; address normalize `<ADDR>`; 15s/case timeout
- Official suite (Linux): **25 identical** (1 vacuous), **1 known-diff (gc)**, **2 not-run (all, main)**

## Remaining / known gaps
1. **Go GC** (intentional): no pace/debt/ephemerons; `gc.lua` known-diff.
2. **main/all.lua** not automated (SIGINT harness).
3. Darwin `*.darwin.expected` may be stale (optional).
4. Warnf plumbing still present for Go-GC `__gc` errors (not a 5.3.4 C API; thin further if unused).
5. Further per-file polish (`lparser`, `lstate`, remaining stdlib comments) vs C line-by-line.

## Paths
- C ref: `/workspace/lua-sources/lua-5.3.4`
- Official tests: `/workspace/lua-sources/lua-5.3.4-tests` → `internal/core/testdata/official/`
- Branch: https://github.com/esrrhs/golua/tree/5.3.4
