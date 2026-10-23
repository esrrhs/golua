# Lua 5.3.4 port status (branch `5.3.4`)

## Done
- Version surface: 5.3.4 / 503 / chunk `0x53`, headers, `_VERSION`, package.cpath `5.3`
- `lopcodes.go`: full 5.3.4 ISA (RK/BITRK, 9-bit B/C)
- Instruction pack/unpack: 5.3 bit layout
- `lcode.go` / `lparser.go` / `lvm` execute: re-translated for 5.3.4
- Fastpaths stubbed (disabled)
- `bit32` library (`lbitlib.go`) wired in `OpenLibs`
- Smoke: arithmetic, tables, field access, functions, for-loops, if/else, recursion, pcall, bit32, string/math OK

## Remaining
1. `ldump`/`lundump` still partially 5.4-shaped (varint sizes, constant tags, lineinfo); need full 5.3 fixed-int format + float/int tag swap for binary chunk interchange with PUC 5.3.4
2. Strip warn / userdatauv / `<const>`/`<close>` public leftovers cleanly (stubs remain for API compile)
3. Update `luac` listing / `ldebug` name resolution for 5.3 ops (stubs allow compile)
4. Point official/diff/CI tests at 5.3.4 `testes/`; expect many failures until dump+details match
5. Re-enable VM fastpaths once green
6. README polish after parity

## Paths
- C ref: `/workspace/lua-sources/lua-5.3.4` (built `src/lua`, `src/luac`)
- Branch: https://github.com/esrrhs/golua/tree/5.3.4
