# Lua 5.3.4 port status (branch `5.3.4`)

## Alignment goal
Line-by-line Go translation of PUC-Lua **5.3.4** C (`/workspace/lua-sources/lua-5.3.4/src`).
**Intentional sole behavioral exception:** Go GC replaces `lgc` (finalizers/weak tables approximated).

## How close to 完全对齐
| Layer | Status |
|-------|--------|
| ISA / opcodes / RK / dump-undump | Aligned with 5.3.4 |
| VM semantics (incl. `#`, hooks, TAILCALL→C) | Aligned; differential **83/83** Linux |
| Standard libraries | Aligned for observable behavior (diff + official) |
| `lparser` / `lcode` / `llex` | Functionally matched; structure mostly 1:1 (e.g. `fieldsel`, `semerror`) |
| `lstate` / CallInfo | Behavior matched; **bit layout** still 5.4-style (`CIST_C` vs 5.3 `CIST_LUA`); not ABI-exposed |
| `lgc` | **Not** a line translation — Go runtime GC + finalizer bridge |
| Public C API surface (`lua.h`) | No 5.4-only exports in CGO; Go may still carry `Warning`/`WarnFunc` for `__gc` |
| Official suite | **25 identical**, 1 known-diff (`gc`), 2 not-run (`main`, `all`) |

True byte-for-byte / line-for-line identity with every `.c` file is **not** claimed: Go types, GC, and coroutine goroutines force structural differences. Observable Lua behavior on Linux matches PUC for the differential suite and official non-GC cases.

## Done (matches 5.3.4 C behavior)
- Version: 5.3.4 / 503 / chunk `0x53`, package path `5.3`, copyright 1994–2017
- ISA (47 ops) + RK; dump/undump interchange with PUC `luac`
- No `warn` global, no `coroutine.close`, no `<const>`/`<close>`; `bit32` present
- File MT: `__index = mt`, methods on mt, no `__close`
- `lua_resume(L, from, nargs)` 5.3 signature; OpenLibs …utf8, debug, bit32
- CGO: no always-on 5.4 exports (toclose/userdatauv/closeslot/setcstacklimit/setwarnf/…)
- `cmd/lua`: options match `lua.c` 5.3.4 (no `-W`)
- `debug`: no `setcstacklimit`; `getinfo` default `flnStu`; `gethook` ×3 results
- Table: `hashint` = `hashpow2`; `luaH_getn` = 5.3.4 search (no alimit shrink)
- `OP_TAILCALL` to C falls through to following `RETURN`
- Parser polish: `fieldsel` / `semerror` named as in `lparser.c`; `CIST_CLSRET` removed
- Differential Linux: **83/83 identical**
- Official Linux: **25 identical**, **1 known-diff (gc)**, **2 not-run (all, main)**

## Remaining gaps vs true line-by-line C
1. **Go GC** (intentional): no pace/debt/ephemerons; `gc.lua` known-diff; `__gc` errors via Go `Warning` (5.3 C uses `LUA_ERRGCMM`).
2. **`main` / `all.lua`**: not automated — Ctrl-C / SIGINT child harness unsafe without hang risk; documented skip.
3. **`lstate` CallInfo bits**: 5.4-shaped (`CIST_C`, `CIST_TRAN`); 5.3 `CIST_LEQ` unused (lessequal path elsewhere).
4. **Warnf / `SetWarnFunction`**: Go-only helpers retained for GC `__gc` reporting; not in 5.3.4 `lua.h`.
5. **Darwin** `*.darwin.expected`: optional refresh not done on this Linux host.
6. Further comment/order polish in large files (`lvm`, `ldo`, stdlibs) where Go already matches behavior.

## Paths
- C ref: `/workspace/lua-sources/lua-5.3.4`
- Official tests: `/workspace/lua-sources/lua-5.3.4-tests` → `internal/core/testdata/official/`
- Branch: https://github.com/esrrhs/golua/tree/5.3.4
