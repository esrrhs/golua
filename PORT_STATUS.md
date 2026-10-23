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

## Remaining / known gaps
1. **Official suite** under captured goldens: only ~7 files identical; many still diverge (bitwise-on-string messages, goto/labels, math edge cases, utf8, tpack, sort comparison counts, etc.). Not silent — `TestOfficialSuite` fails until parity improves.
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
