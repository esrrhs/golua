# Lua 5.3.4 port status (branch `5.3.4`)

## Alignment goal
Treat this branch as a line-by-line Go translation of PUC-Lua **5.3.4** C sources
(`/workspace/lua-sources/lua-5.3.4/src`), same spirit as the original 5.4.9 port.
**Intentional sole exception:** Go GC replaces `lgc` (finalizers/weak tables approximated).

## Done (matches 5.3.4 C)
- Version surface: 5.3.4 / 503 / chunk `0x53`, `_VERSION`, package path `5.3`, copyright 1994-2017
- Full 5.3.4 ISA (47 ops), RK encoding, codegen, parser, VM; dump/undump interchange with PUC
- `bit32` present; **no** `warn`, **no** `coroutine.close`, **no** `<const>`/`<close>`
- File handles: `__index = mt`, methods on mt, **no `__close`** (createmeta as in liolib.c)
- `lua_resume(L, from, nargs)` — 5.3 signature (no nresults out-param)
- OpenLibs order: …utf8, **debug**, bit32 (matches linit.c with LUA_COMPAT_BITLIB)
- Public `include/lua.h` is 5.3.4; optional 5.4 names only behind `LUA_COMPAT_54` (off)
- CGO no longer exports warn/toclose/userdatauv/closeslot/closethread/resetthread/getiuservalue
- Negative 5.4 opcode stubs and `compat54_stubs.go` removed
- TBC / to-be-closed runtime paths removed (`luaF_close` = close upvalues only)
- Official `testes/` = 5.3.4 suite; Linux goldens; suite tip previously **25/28 identical**

## Remaining / known gaps
1. **Go GC** (intentional): no collector pace/debt/ephemerons as in `lgc`; `gc.lua` known-diff.
2. **Official suite**: `main.lua` / `all.lua` not run (SIGINT harness); Darwin goldens may be stale.
3. **VM fastpaths** stubbed (`lvm_fastcall.go`) — rewrite for 5.3 RK before enabling.
4. **Edge skips**: varinfo `(constant '…')` for bitwise RK; numeric for + NaN step; some third-party goldens.
5. Dormant Go helpers may remain for warning/resetthread internals unused by the 5.3 API surface.
6. `luaD_closeprotected` kept as thin upvalue-close wrapper where call sites still name it.

## Paths
- C ref: `/workspace/lua-sources/lua-5.3.4`
- Official tests: `/workspace/lua-sources/lua-5.3.4-tests` → `internal/core/testdata/official/`
- Branch: https://github.com/esrrhs/golua/tree/5.3.4
