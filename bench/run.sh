#!/bin/sh
# Time every workload under one or more interpreters.
#
#     sh bench/run.sh /tmp/refcap/lua ./lua
#
# Run the interpreters one at a time and read the rows down: the point
# is to measure the same shape of work twice, not to run two
# interpreters against each other while both compete for the machine.
# Each workload is run REPS times (three, by default) and the fastest
# wall time is kept, because a single run moves with whatever else the
# machine is doing.  REPS, NODES and ROUNDS come from the environment.
#
# 'time -l' here is the BSD form, as macOS has it: the number beside
# 'real' is seconds, and the one beside 'maximum resident set size' is
# bytes.  The per-node figure is not something the command line can
# see -- it comes from bench/node_size.lua, which asks the interpreter
# to count for itself.
set -e

cd "$(dirname "$0")/.."

reps=${REPS:-3}
nodes=${NODES:-2000000}
rounds=${ROUNDS:-20}

if [ $# -eq 0 ]; then
    echo "usage: sh bench/run.sh <interpreter> [interpreter ...]" >&2
    exit 2
fi

# walltime runs one workload REPS times and prints the fastest 'real'
# time it saw, in seconds.
walltime() {
    interp=$1
    shift
    times=
    i=0
    while [ $i -lt "$reps" ]; do
        t=$(/usr/bin/time -l "$interp" "$@" 2>&1 >/dev/null |
            awk '/ real/{print $1}')
        times="$times${times:+ }${t:-nan}"
        i=$((i + 1))
    done
    printf '%s\n' "$times" | sort -g | awk 'NR == 1 {v = $1} END {print v}'
}

for interp in "$@"; do
    echo "== $interp =="
    printf 'fib(28)        : %ss\n' "$(walltime "$interp" bench/fib.lua)"
    printf 'arith(2e7)     : %ss\n' "$(walltime "$interp" bench/arith.lua)"
    printf 'sort(2e5)      : %ss\n' "$(walltime "$interp" bench/sort.lua)"
    printf 'strings(2e4)   : %ss\n' "$(walltime "$interp" bench/strings.lua)"
    printf 'build %s : %ss\n' "$nodes" "$(walltime "$interp" bench/mesh_gc.lua "$nodes" 0)"
    printf '+%s collects   : %ss\n' "$rounds" "$(walltime "$interp" bench/mesh_gc.lua "$nodes" "$rounds")"
    printf 'one node       : %s B\n' "$("$interp" bench/node_size.lua 1000000)"
    printf 'peak memory    : %s\n' \
        "$(/usr/bin/time -l "$interp" bench/mesh_gc.lua "$nodes" 0 2>&1 >/dev/null |
            awk '/maximum resident/{printf "%.0f MB", $1 / 1048576}')"
done
