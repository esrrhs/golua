# Lua 5.3.4 port status (branch `5.3.4`)

## Alignment goal
Line-by-line Go translation of PUC-Lua **5.3.4** C (`/workspace/lua-sources/lua-5.3.4/src`).
**Intentional sole behavioral exception:** Go GC replaces `lgc` (finalizers/weak tables approximated).

## How close to 完全对齐
| Layer | Status |
|-------|--------|
| ISA / opcodes / RK / dump-undump | Aligned with 5.3.4 |
| VM semantics | Aligned; differential **83/83** Linux |
| Standard libraries | Aligned (diff + official non-GC) |
| `CallInfo` status bits | **Match `lstate.h` 5.3.4** (`CIST_LUA`/`HOOKED`/`FRESH`/`LEQ`/`FIN` layout) |
| `Proto` | Match 5.3 fields (+ Go-only `fastLeaf` cache); no 5.4 `abslineinfo` |
| `lparser` / `lcode` / `llex` | Functionally 1:1 (`fieldsel`, `semerror`, …) |
| `global_State` / GC lists | **Not** line-translated — Go runtime GC + finalizer bridge |
| Public C API (`lua.h`) | No 5.4-only CGO exports; Go `Warning`/`WarnFunc` kept for `__gc` |
| Official suite | **25 identical**, 1 known-diff (`gc`), 2 not-run (`main`, `all`) |

True byte-for-byte identity with every `.c` file is not claimed (Go types, GC, coroutine goroutines). Observable Linux Lua behavior matches PUC for differential + official non-GC cases.

## Done (matches 5.3.4 C behavior / structure)
- Version 5.3.4 / ISA 47 ops + RK; dump/undump ↔ PUC `luac`
- No `warn` global, no `coroutine.close`, no `<const>`/`<close>`; `bit32` present
- `CallInfo` bits = `lstate.h` 5.3.4; `isLua` = `CIST_LUA`; no `CIST_C`/`CIST_TRAN`/`CIST_CLSRET`
- `CIST_LEQ` set around `__lt`-for-`__le` (COMPAT_LT_LE)
- Table `hashint`=`hashpow2`; `luaH_getn` 5.3.4; TAILCALL→C falls through to RETURN
- Differential Linux **83/83**; Official **25 / 1 known-diff / 2 not-run**

## Remaining gaps vs true line-by-line C
1. **Go GC** (intentional): no pace/debt/ephemerons/allgc lists; `gc.lua` known-diff; `__gc` errors via `Warning` (C: `LUA_ERRGCMM`).
2. **`main`/`all.lua`**: SIGINT harness not automated (documented skip).
3. **CallInfo** still carries Go-only fields (`trap`, `ftransfer`/`ntransfer`, `goCont`, …) that 5.3 packs differently or omits — bits match; struct shape is Go.
4. **`Proto.fastLeaf`**: Go-only VM fastpath cache.
5. Darwin `*.darwin.expected` not refreshed on this Linux host.
6. Further comment/order polish in large files where behavior already matches.

## Paths
- C ref: `/workspace/lua-sources/lua-5.3.4`
- Official tests: `/workspace/lua-sources/lua-5.3.4-tests` → `internal/core/testdata/official/`
- Branch: https://github.com/esrrhs/golua/tree/5.3.4
