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
| Official suite (non-GC) | Yes | **26 identical** of 28; `gc`/`all` known-diff (`all` with `_port`); `main` enabled |
| Third-party / LPeg | Yes | LPeg goldens vs PUC **5.3.4**; `TestLPeg` enabled |
| Stand-alone SIGINT | Yes | `cmd/lua` `laction`/`lstop` equivalent; process-group kill in harness |
| `lparser` / `lcode` / `llex` | Yes (behavior) | Structure ~1:1 |
| File headers / docs | Yes | Point at 5.3.4; bench tables vs PUC **5.3.4** |
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
- LPeg `expected.txt` verified under PUC 5.3.4; `TestLPeg` enabled
- Stand-alone SIGINT: first INT → `SetInterruptHook` ("interrupted!"); second INT → process exit (Go cannot restore kernel SIG_DFL usefully). Official harness uses Setpgid + group kill on timeout.
- Bench tables refreshed vs PUC **5.3.4** (Intel Xeon, Go 1.27 cgo, `MIN_MS=600 REPS=7`)

## Still open (do not thrash)
1. Go GC (pace/debt/ephemerons/`gc.lua` known-diff; `all.lua` needs `_port` for the same `dosteps(0)` reason)
2. Darwin golden refresh — optional; Linux host cannot rewrite `*.darwin.expected`; macos CI owns them
3. Further comment/order polish where behavior already matches
4. Optional: longer `MIN_MS`/`REPS` bench pass on beefier hardware

## Paths
- C ref: `/workspace/lua-sources/lua-5.3.4`
- Official tests: `/workspace/lua-sources/lua-5.3.4-tests` → `internal/core/testdata/official/`
- Branch: https://github.com/esrrhs/golua/tree/5.3.4
