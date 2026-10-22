#!/bin/sh
# Compare interpreters the way Google Benchmark compares two runs.
#
#     sh bench/compare.sh /usr/local/bin/lua ./lua
#     MIN_MS=500 REPS=9 sh bench/compare.sh /usr/local/bin/lua ./lua
#     ONLY="fib alloc" sh bench/compare.sh /usr/local/bin/lua ./lua
#
# Each workload is one call of the function in bench/suite.lua.  A single
# call can finish in a few milliseconds, and that number is mostly process
# startup, so it is not timed.  For each interpreter the call is stacked
# until one process runs at least MIN_MS (default 1000).  The stack count
# is chosen per interpreter, the same way Google Benchmark chooses its
# iteration count: both sides accumulate enough time for the startup to
# be a small fraction of the run, and the thing that is compared is the
# mean time per call.
#
# That timed process is then repeated REPS times (default 9, at most 25).
# The row reports the mean, the coefficient of variation, the absolute
# difference against the first interpreter, the relative difference
# (other - baseline) / baseline, and the ratio.  The p-value is an exact
# two-sided Mann-Whitney U test on the per-call samples.  `noise` means
# the test did not reject equality at ALPHA (default 0.05): the
# difference is not distinguishable from the scatter of the repetitions.
# `short` means that side never reached MIN_MS, so the mean still sits
# on the startup floor and the difference is not a measurement of the
# workload.
#
# The clock is read outside the process.  os.clock is CPU time in PUC-Lua
# and wall time since startup in this port, so a timestamp taken in Lua
# would compare two different things.  Interpreters are run one at a
# time.  ONLY restricts the rows; names come from `suite.lua list`, plus
# `startup`, which is the process itself and is not stacked.
set -eu
export LC_ALL=C

cd "$(dirname "$0")/.."

min_ms=${MIN_MS:-1000}
reps=${REPS:-9}
max_n=${MAX_N:-100000000}
alpha=${ALPHA:-0.05}

if [ $# -lt 1 ]; then
	echo "usage: sh bench/compare.sh <interpreter> [interpreter ...]" >&2
	exit 2
fi
case $min_ms in
	''|*[!0-9]*) echo "MIN_MS must be a positive integer (milliseconds)" >&2; exit 2 ;;
esac
case $reps in
	''|*[!0-9]*) echo "REPS must be an integer from 2 to 25" >&2; exit 2 ;;
esac
case $max_n in
	''|*[!0-9]*) echo "MAX_N must be a positive integer" >&2; exit 2 ;;
esac
if [ "$min_ms" -lt 1 ] || [ "$reps" -lt 2 ] || [ "$reps" -gt 25 ] || [ "$max_n" -lt 1 ]; then
	echo "MIN_MS >= 1, REPS from 2 to 25, MAX_N >= 1" >&2
	exit 2
fi

min_ns=$((min_ms * 1000000))
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

if ! "$1" bench/suite.lua list >"$tmpdir/list"; then
	echo "cannot list workloads: $1" >&2
	exit 1
fi
workloads="startup $(tr '\n' ' ' <"$tmpdir/list")"

selected=
if [ -n "${ONLY:-}" ]; then
	for want in $ONLY; do
		found=0
		for name in $workloads; do
			if [ "$name" = "$want" ]; then
				found=1
			fi
		done
		if [ "$found" -eq 0 ]; then
			echo "unknown workload: $want" >&2
			exit 2
		fi
		selected="$selected $want"
	done
else
	selected=$workloads
fi

ninterp=0
for interp in "$@"; do
	ninterp=$((ninterp + 1))
	eval "interp_$ninterp=\$interp"
	eval "base_$ninterp=\$(basename \"\$interp\")"
	eval "label_$ninterp=\$base_$ninterp"
done
# Rename after counting.  Doing it in the same pass makes the second
# "lua" look unique once the first has become "lua#1".
i=1
while [ "$i" -le "$ninterp" ]; do
	eval "lab=\$base_$i"
	dups=0
	j=1
	while [ "$j" -le "$ninterp" ]; do
		eval "other=\$base_$j"
		if [ "$lab" = "$other" ]; then
			dups=$((dups + 1))
		fi
		j=$((j + 1))
	done
	if [ "$dups" -gt 1 ]; then
		eval "label_$i=\${lab}#$i"
	fi
	i=$((i + 1))
done

# run_once leaves the elapsed nanoseconds in _ns.
run_once() {
	_start=$(date +%s%N)
	if [ "$2" = startup ]; then
		"$1" -e 'print(0)' >"$4"
	else
		"$1" bench/suite.lua "$2" "$3" >"$4"
	fi
	_status=$?
	_end=$(date +%s%N)
	if [ "$_status" -ne 0 ]; then
		echo "failed: $1 $2 $3" >&2
		exit 1
	fi
	_ns=$((_end - _start))
}

# Next stack count.  Grow by the ratio still missing, but never by more
# than 10x in one step: a single noisy pilot must not schedule an hour.
scale_n() {
	_cur=$1
	_got=$2
	if [ "$_got" -le 0 ]; then
		_mul=10
	else
		_mul=$((min_ns / _got))
		if [ "$_mul" -lt 2 ]; then
			_mul=2
		fi
		if [ "$_mul" -gt 10 ]; then
			_mul=10
		fi
	fi
	if [ "$_cur" -ge "$max_n" ] || [ "$_cur" -gt $((max_n / _mul)) ]; then
		echo "$max_n"
	else
		echo $((_cur * _mul))
	fi
}

median_file() {
	sort -n "$1" | awk '{ a[NR] = $1 }
		END {
			if (NR == 0) { print 0; exit }
			if (NR % 2 == 1) print a[(NR + 1) / 2]
			else print int((a[NR / 2] + a[NR / 2 + 1]) / 2)
		}'
}

collect() {
	_file=$1
	: >"$_file"
	_k=0
	while [ "$_k" -lt "$reps" ]; do
		run_once "$interp" "$name" "$n" "$tmpdir/out"
		echo "$_ns" >>"$_file"
		_k=$((_k + 1))
	done
}

echo "# min ${min_ms} ms, ${reps} repetitions, two-sided Mann-Whitney U, alpha ${alpha}"
echo "# time is the mean per stacked call; delta is (side - first) / first"
echo "# noise: p >= alpha, the difference is within the scatter of the repetitions"
echo "# short: that side never reached the minimum time, so startup still dominates"
i=1
while [ "$i" -le "$ninterp" ]; do
	eval "echo \"# $i = \$interp_$i\""
	i=$((i + 1))
done
awk -v mode=header -f bench/compare.awk

for name in $selected; do
	echo "measuring $name" >&2
	i=0
	diff=0
	while [ "$i" -lt "$ninterp" ]; do
		i=$((i + 1))
		eval "interp=\$interp_$i"
		if [ "$name" = startup ]; then
			if ! "$interp" -e 'print(0)' >"$tmpdir/check.$i"; then
				echo "failed: $interp startup" >&2
				exit 1
			fi
		else
			if ! "$interp" bench/suite.lua "$name" 1 >"$tmpdir/check.$i"; then
				echo "failed: $interp $name" >&2
				exit 1
			fi
		fi
		if [ "$i" -gt 1 ] && ! cmp -s "$tmpdir/check.1" "$tmpdir/check.$i"; then
			diff=1
		fi
	done

	{
		echo "alpha $alpha"
		echo "min_ns $min_ns"
		echo "workload $name"
		if [ "$diff" -eq 1 ]; then
			echo "kind diff"
		else
			if [ "$name" = startup ]; then
				echo "kind startup"
			else
				echo "kind work"
			fi
		fi
	} >"$tmpdir/job"

	if [ "$diff" -eq 0 ]; then
		i=0
		while [ "$i" -lt "$ninterp" ]; do
			i=$((i + 1))
			eval "interp=\$interp_$i"
			eval "lab=\$label_$i"
			if [ "$name" = startup ]; then
				n=0
				collect "$tmpdir/samples"
			else
				n=1
				_steps=0
				while [ "$n" -lt "$max_n" ] && [ "$_steps" -lt 24 ]; do
					run_once "$interp" "$name" "$n" "$tmpdir/out"
					if [ "$_ns" -ge "$min_ns" ]; then
						break
					fi
					_next=$(scale_n "$n" "$_ns")
					if [ "$_next" -le "$n" ]; then
						break
					fi
					n=$_next
					_steps=$((_steps + 1))
				done
				_corrections=0
				while true; do
					collect "$tmpdir/samples"
					_med=$(median_file "$tmpdir/samples")
					if [ "$_med" -ge "$min_ns" ] || [ "$n" -ge "$max_n" ] || [ "$_corrections" -ge 3 ]; then
						break
					fi
					_next=$(scale_n "$n" "$_med")
					if [ "$_next" -le "$n" ]; then
						break
					fi
					n=$_next
					_corrections=$((_corrections + 1))
				done
			fi
			echo "side $lab $n" >>"$tmpdir/job"
			cat "$tmpdir/samples" >>"$tmpdir/job"
		done
	fi
	awk -f bench/compare.awk "$tmpdir/job"
done
