# Lua 5.3.4 port status (branch `5.3.4`)

## Alignment goal
Line-by-line Go translation of PUC-Lua **5.3.4** C (`/workspace/lua-sources/lua-5.3.4/src`).
**Intentional sole behavioral exception:** Go GC replaces `lgc`.

## Final alignment scorecard

| Area | Match 5.3.4 C? | Notes |
|------|----------------|-------|
| Version / chunk / copyright | Yes | 5.3.4 / 503 / `0x53` / 1994–2017 |
| ISA (47 ops) + RK | Yes | No 5.4 opcodes |
| CallInfo **status bits** | Yes | `lstate.h` layout (`CIST_LUA`…`CIST_FIN`) |
| `luaH_getn` / `hashint` | Yes | 5.3.4 binary+unbound; `hashpow2` |
| Stdlibs observable behavior | Yes | Differential **83/83** Linux |
| Official suite (non-GC) | Yes | **25 identical** of 28; `main`/`all` not-run (SIGINT); `gc` known-diff |
| Third-party / LPeg | Yes | LPeg goldens re-captured vs PUC **5.3.4**; `TestLPeg` enabled in CI |
| `lparser` / `lcode` / `llex` | Yes (behavior) | Structure ~1:1 |
| File headers / docs | Yes | Point at 5.3.4 (sweep done); README/bench label old 5.4.7 ratios |
| **Go GC / `lgc`** | **No (intentional)** | Go runtime + finalizer bridge |
| CallInfo **extra fields** | Go adaptation | `trap`, `ftransfer`/`ntransfer`, `goCont`, … |
| `Proto.fastLeaf` | Go adaptation | VM leaf fastpath cache |
| `Table.alimit` name | Go adaptation | ≡ C `sizearray` (always real; no 5.4 shrink) |
| `Table.weakMode` / `finalized` | Go adaptation | Weak tables / `__gc` without C GC lists |
| `WarnFunc` / `Warning` | Go adaptation | Not in 5.3.4 `lua.h`; used for `__gc` errors (C: `LUA_ERRGCMM`) |
| `global_State` GC lists | Go adaptation | No `allgc`/ephemeron walk |

**Verdict:** Observable Lua on Linux matches PUC 5.3.4 for the differential suite and official non-GC cases. Remaining differences are documented Go GC / VM adaptations, not leftover 5.4 APIs.

## Done highlights
- No `warn` global, no `coroutine.close`, no `<const>`/`<close>`, no TBC; `bit32` present
- CGO: no always-on 5.4 exports (`toclose`, `setcstacklimit`, `setwarnf`, …)
- `OP_TAILCALL`→C falls through to `RETURN`; utf8/`require`/io/os/debug edges aligned
- Dead `setnorealasize` / `CIST_C`/`TRAN`/`CLSRET` / Proto `abslineinfo` removed
- LPeg `expected.txt` re-recorded under PUC 5.3.4; blanket `t.Skip` removed
- Root / bench READMEs: CI Lua **5.3.4**, official **28** files / **25** identical, license testes 5.3.4; perf tables labeled as older 5.4.7 ratios

## Still open (do not thrash)
1. Go GC (pace/debt/ephemerons/`gc.lua` known-diff)
2. `main`/`all` SIGINT harness — still skipped; needs stand-alone `laction`-equivalent + safe process-group kill before un-skip (see `officialSkip` in `official_test.go`)
3. Darwin golden refresh — optional; Linux host cannot rewrite `*.darwin.expected`; macos CI owns them (linux goldens already committed)
4. Further comment/order polish where behavior already matches
5. Optional: re-run `bench/compare.sh` vs PUC 5.3.4 and replace the labeled 5.4.7 ratio tables

## Paths
- C ref: `/workspace/lua-sources/lua-5.3.4`
- Official tests: `/workspace/lua-sources/lua-5.3.4-tests` → `internal/core/testdata/official/`
- Branch: https://github.com/esrrhs/golua/tree/5.3.4
