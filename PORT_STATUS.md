# Lua 5.3.4 port status (branch `5.3.4`)

## Done
- Version surface: 5.3.4 / 503 / chunk `0x53`, `_VERSION`, package.cpath `5.3`, LUA_COPYRIGHT 1994-2017
- Full 5.3.4 ISA, codegen, parser, VM execute; bit32; dump/undump interchange with PUC 5.3.4
- `luac -l` / `ldebug` name resolution for 5.3 RK; `adjust_varargs` + `CallInfo.base`
- CONCAT top fix; parser `nCcalls&0xffff` mask; int/float K-cache separation (lightuserdata keys for ints)
- Public `include/lua.h` matches 5.3.4 by default; Lua 5.4 names behind **`LUA_COMPAT_54`** (off). Cgo still exports no-op/compat stubs for ABI.
- Conformance / corolib / close tests for `<close>` / `<const>` / `coroutine.close` skipped or adapted
- Official `testes/` = 5.3.4 suite; CI + `LUA_REF` → lua-5.3.4; **Linux goldens captured** (`*.linux.expected`)
- Regression: float literal `3.0` stays float (`TestFloatLiteralTag`)
- Arith/bitwise coerce strings via `luaV_tonumber_` / `tointeger` (5.3), not 5.4 `*ns`
- `test_then_block` goto/break path matches PUC 5.3.4 (`jf` skip + no corrupt patch of `v.t`); goto.lua compiles (`TestGotoIfThenElseifLabels`)
- Bitwise TM errors: `tonumber`/`tointeger` (not `ttisnumber`/`tointegerns`) so `2.3 ~ '0.0'` → no integer representation
- `math.random` 5.3 empty/too-large interval checks (no `random(0)` full-int); `utf8.char`/`decode` MAXUNICODE; utf8 msgs; `string.unpack` pos 0 via 5.3 `posrelat`
- `string.format` scanformat errors (too long / repeated flags / invalid option); no 5.4 string arith metamethods (varinfo via trybinTM)
- `print` calls global `tostring` like 5.3.4 (not luaL_tolstring)
- `funcname` fieldsel uses `luaK_exp2anyregup` before index (method/table function assign)
- Restored `gcAfterAlloc` on NEWTABLE/CONCAT/CLOSURE (weak busy-wait)
- Proto.cache / getcached for closure equality
- `debug.upvalueid` always ArgChecks invalid index
- pretailcall sets `ci.base` and runs `adjustVarargs` for 5.3 vararg protos (__call chains)
- `forprep53` float path uses `luaV_tonumber_` (string coerce), not tonumberns
- Parser `enterlevel` reports "too many C levels" (checklimit), not runtime "C stack overflow"
- Line/call hooks: no VARARGPREP skip; honour `ci.trap` from setTraps mid-chunk; `npc==0` line fire
- `debug.getlocal`/`setlocal` varargs: `(*vararg)` slots between func and base; `(*temporary)` / `(*no name)`
- `getfuncname` 5.3: CIST_FIN on queried frame; metamethod names keep `__` prefix
- `luaG_callerror` uses varinfo (5.3), not 5.4 frame-entry naming
- `math.random`/`randomseed`: POSIX `random()`/`srandom()` (5.3), not xoshiro; sort comparison counts match
- officialNormalise: short-circuit GLOB1 digit (`os.time()%2`)
- package: `LUA_*_5_3` versioned env; `;;` → `;`+default+`;` (setpath); preload searcher returns 1 value (not `:preload:`)

## Remaining / known gaps
1. **Official suite**: target **24 identical** of 26 run (1 vacuous api). Not run: main, all (Ctrl-C harness via main), heavy. Known-diff when run: gc, literals.
2. **Darwin** `*.darwin.expected` still from 5.4.9 era — re-capture on macOS with PUC 5.3.4.
3. **VM fastpaths** remain stubbed (`lvm_fastcall.go`): leaf recognition was 5.4-opcode-shaped; rewrite for 5.3 RK before enabling.
4. **varinfo** for bitwise RK ops does not yet append `(constant '…')` (test skipped).
5. **Numeric for + NaN step** not matching PUC yet (test skipped).
6. Third-party goldens (LPeg, etc.) captured from 5.4.9 — LPeg test skipped pending re-capture; fuzz corpora that embed `<close>`/`coroutine.close` are harmless under LUA_REF=5.3.4 (both sides error) but should be trimmed when convenient.
7. `lua_resume` C signature keeps 5.4 out-param form (documented in lua.h).
8. README polish after official suite is closer to green.

## Paths
- C ref: `/workspace/lua-sources/lua-5.3.4` (`src/lua`, `src/luac`)
- Official tests: `/workspace/lua-sources/lua-5.3.4-tests` → `internal/core/testdata/official/`
- Branch: https://github.com/esrrhs/golua/tree/5.3.4
