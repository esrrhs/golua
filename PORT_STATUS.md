# Lua 5.3.4 port status (branch `5.3.4`)

## Done
- Version surface: 5.3.4 / 503 / chunk `0x53`, headers, `_VERSION`, package.cpath `5.3`, LUA_COPYRIGHT
- `lopcodes.go`: full 5.3.4 ISA (RK/BITRK, 9-bit B/C)
- Instruction pack/unpack: 5.3 bit layout
- `lcode.go` / `lparser.go` / `lvm` execute: re-translated for 5.3.4
- `ldump.go` / `lundump.go`: 5.3.4 fixed-size format (int/size_t sizes in header, string size byte/`0xFF`+size_t, dump tags NUMFLT=3 / NUMINT=19, upvalues without kind, absolute lineinfo)
- Binary chunk interchange with PUC 5.3.4 `lua`/`luac` verified both directions (same size on smoke chunk)
- `bit32` library (`lbitlib.go`) wired in `OpenLibs`
- `luac -l` listing matches PUC 5.3.4 PrintCode (MYK / ISK)
- `ldebug` name resolution ported to 5.3 RK / getobjname / funcnamefromcode
- `adjust_varargs` + `CallInfo.base` (5.3 vararg layout); CONCAT no longer clobbers `L.top` via savestate
- Official `testes/` retargeted to Lua 5.3.4 suite; CI downloads `lua-5.3.4`
- Smoke: arithmetic, tables, fields, functions, for-loops, if/else, recursion, pcall, bit32, string/math, string.dump, vararg OK

## Remaining / known gaps
1. 5.4 API stubs retained for ABI (`lua_warning`, `lua_toclose`, `lua_newuserdatauv`, `lua_getiuservalue`, …) — annotated in `include/lua.h`; runtime is no-op / single-uservalue
2. Official/diff goldens need re-capture under `LUA_REF` pointing at PUC 5.3.4 (linux expected files not checked in yet; darwin ones are stale 5.4.9)
3. VM fastpaths still stubbed — re-enable once official suite is green
4. Some third-party / fuzz expected files still mention 5.4.9
5. README polish after parity

## Paths
- C ref: `/workspace/lua-sources/lua-5.3.4` (built `src/lua`, `src/luac`)
- Official tests tarball: `/workspace/lua-sources/lua-5.3.4-tests`
- Branch: https://github.com/esrrhs/golua/tree/5.3.4
