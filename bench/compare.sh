#!/bin/sh
# Wall-time comparison of bench/suite.lua across interpreters.
#
#     sh bench/compare.sh /usr/local/bin/lua ./lua
#
# The first interpreter is the baseline.  Each workload runs REPS times
# (three by default) and the fastest wall time is kept.  The line the
# script prints is compared across interpreters; a row whose output
# differs is marked DIFF and its ratio is not meaningful.
#
# The clock is read outside the process, with nanosecond timestamps.
# os.clock is CPU time in PUC-Lua and wall time since startup in this
# port, so a timestamp taken in Lua would compare two different things.
set -e

cd "$(dirname "$0")/.."

reps=${REPS:-3}

if [ $# -lt 1 ]; then
	echo "usage: sh bench/compare.sh <interpreter> [interpreter ...]" >&2
	exit 2
fi

# Names in the order bench/suite.lua lists them, plus process startup,
# which never loads the suite.
workloads="startup $($1 bench/suite.lua list)"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

# best <interpreter> <workload> <stamp> prints the fastest of REPS runs,
# in seconds with three decimals, and saves that run's stdout.
best() {
	interp=$1
	name=$2
	stamp=$3
	bestns=
	i=0
	while [ "$i" -lt "$reps" ]; do
		out="$tmpdir/out"
		start=$(date +%s%N)
		if [ "$name" = startup ]; then
			"$interp" -e 'print(0)' >"$out"
		else
			"$interp" bench/suite.lua "$name" >"$out"
		fi
		end=$(date +%s%N)
		ns=$((end - start))
		if [ -z "$bestns" ] || [ "$ns" -lt "$bestns" ]; then
			bestns=$ns
			cp "$out" "$tmpdir/best.$stamp"
		fi
		i=$((i + 1))
	done
	awk -v ns="$bestns" 'BEGIN { printf "%.3f", ns / 1000000000 }'
}

n=0
for interp in "$@"; do
	n=$((n + 1))
	eval "label_$n=\$(basename \"\$interp\")"
	echo "# $n = $interp"
done

header=$(printf '%-14s' workload)
sep=$(printf '%-14s' --------------)
i=1
while [ "$i" -le "$n" ]; do
	eval "lab=\$label_$i"
	header="$header $(printf '%10s' "$lab")"
	sep="$sep $(printf '%10s' ----------)"
	i=$((i + 1))
done
echo "$header   ratio"
echo "$sep   -----"

for name in $workloads; do
	line=$(printf '%-14s' "$name")
	base=
	last=
	diff=0
	i=0
	for interp in "$@"; do
		i=$((i + 1))
		t=$(best "$interp" "$name" "$i")
		line="$line $(printf '%10s' "$t")"
		if [ -z "$base" ]; then
			base=$t
		fi
		last=$t
		if [ "$i" -gt 1 ] && ! cmp -s "$tmpdir/best.1" "$tmpdir/best.$i"; then
			diff=1
		fi
	done
	if [ "$diff" -ne 0 ]; then
		ratio=DIFF
	else
		ratio=$(awk -v a="$last" -v b="$base" 'BEGIN {
			if (b + 0 == 0) print "n/a"
			else printf "%.2fx", a / b
		}')
	fi
	echo "$line   $ratio"
done
