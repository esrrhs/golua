# One workload, already measured, turned into a row.
#
# The shell writes the record.  The point estimate is the mean time per
# call (the whole process, divided by the number of stacked calls),
# which is what Google Benchmark reports.  The p-value is an exact
# two-sided Mann-Whitney U test on those per-call times, the same test
# its compare tool uses to decide whether a difference is noise.
#
#   awk -v mode=header -f bench/compare.awk
#   awk -v mode=selftest -f bench/compare.awk
#   awk -f bench/compare.awk jobfile
BEGIN {
	if (mode == "header") {
		printf "%-14s %-12s %10s %13s %7s %13s %9s %9s %8s %s\n",
			"workload", "side", "calls", "time", "cv",
			"diff", "delta", "ratio", "p", "note"
		exit
	}
	if (mode == "selftest") {
		selftest()
		exit failed
	}
}

$1 == "kind" { kind = $2; next }
$1 == "alpha" { alpha = $2 + 0; next }
$1 == "min_ns" { min_ns = $2 + 0; next }
$1 == "workload" { workload = $2; next }
$1 == "side" {
	nsides++
	label[nsides] = $2
	calls[nsides] = $3 + 0
	scount[nsides] = 0
	curside = nsides
	next
}
curside > 0 && $1 ~ /^[0-9]+$/ {
	scount[curside]++
	samples[curside, scount[curside]] = $1 + 0
	next
}

END {
	if (mode == "header")
		exit 0
	if (mode == "selftest")
		exit failed
	if (alpha == 0)
		alpha = 0.05
	if (kind == "diff") {
		printf "%-14s %-12s %10s %13s %7s %13s %9s %9s %8s %s\n",
			workload, "-", "-", "-", "-", "-", "-", "-", "-",
			"DIFF stdout disagrees"
		exit
	}
	for (s = 1; s <= nsides; s++)
		summarize(s)
	for (s = 1; s <= nsides; s++)
		print_side(s)
}

function per_call(side, total) {
	if (calls[side] <= 0)
		return total
	return total / calls[side]
}

function summarize(side,    n, i, sum, mean, var, per) {
	n = scount[side]
	sum = 0
	for (i = 1; i <= n; i++) {
		per = per_call(side, samples[side, i])
		perv[side, i] = per
		sum += per
	}
	mean = (n > 0) ? sum / n : 0
	mean_of[side] = mean
	var = 0
	if (n > 1) {
		for (i = 1; i <= n; i++)
			var += (perv[side, i] - mean) * (perv[side, i] - mean)
		var /= (n - 1)
	}
	if (n > 1 && mean != 0)
		cv_of[side] = sqrt(var) / mean * 100
	else
		cv_of[side] = -1
	short_of[side] = 0
	if (kind != "startup" && n > 0 && side_median(side) < min_ns)
		short_of[side] = 1
}

# Median of the raw process times, used only to decide whether the
# stacked run ever got long enough.
function side_median(side,    n, i, j, v) {
	n = scount[side]
	for (i = 1; i <= n; i++)
		medarr[i] = samples[side, i]
	for (i = 2; i <= n; i++) {
		v = medarr[i]
		j = i - 1
		while (j >= 1 && medarr[j] > v) {
			medarr[j + 1] = medarr[j]
			j--
		}
		medarr[j + 1] = v
	}
	if (n % 2 == 1)
		return medarr[(n + 1) / 2]
	return (medarr[n / 2] + medarr[n / 2 + 1]) / 2
}

function print_side(side,    note, diff, delta, ratio, p, calls_s, cv_s) {
	note = ""
	diff = ""
	delta = ""
	ratio = ""
	p = ""
	if (short_of[side])
		note = add_note(note, "short")
	else if (side > 1 && short_of[1])
		note = add_note(note, "baseline-short")
	if (side > 1) {
		if (mean_of[1] == 0) {
			delta = "n/a"
			ratio = "n/a"
			diff = "n/a"
		} else {
			diff = fmt_time(mean_of[side] - mean_of[1])
			delta = fmt_delta(mean_of[side] / mean_of[1] - 1)
			ratio = fmt_ratio(mean_of[side] / mean_of[1])
		}
		if (scount[1] < 2 || scount[side] < 2) {
			p = "-"
			note = add_note(note, "once")
		} else {
			p = utest_p(1, side)
			if (p + 0 >= alpha)
				note = add_note(note, "noise")
			p = fmt_p(p)
		}
	}
	if (calls[side] <= 0)
		calls_s = "-"
	else
		calls_s = sprintf("%d", calls[side])
	if (cv_of[side] < 0)
		cv_s = "-"
	else
		cv_s = sprintf("%.1f%%", cv_of[side])
	printf "%-14s %-12s %10s %13s %7s %13s %9s %9s %8s %s\n",
		workload, label[side], calls_s, fmt_time(mean_of[side]), cv_s,
		diff, delta, ratio, p, note
}

function add_note(note, extra) {
	if (note == "")
		return extra
	if (index("," note ",", "," extra ",") > 0)
		return note
	return note "," extra
}

function fmt_time(ns,    sign, v, u) {
	sign = ""
	if (ns < 0) {
		sign = "-"
		ns = -ns
	}
	if (ns < 1000) {
		v = ns
		u = "ns"
	} else if (ns < 1000000) {
		v = ns / 1000
		u = "us"
	} else if (ns < 1000000000) {
		v = ns / 1000000
		u = "ms"
	} else {
		v = ns / 1000000000
		u = "s"
	}
	return sprintf("%s%.3f %s", sign, v, u)
}

function fmt_delta(d) {
	if (d > 99 || d < -99)
		return sprintf("%+.3g", d)
	return sprintf("%+.4f", d)
}

function fmt_ratio(r) {
	if (r >= 0.01 && r < 100)
		return sprintf("%.2fx", r)
	return sprintf("%.3gx", r)
}

function fmt_p(p) {
	if (p < 0.0001)
		return "<0.0001"
	return sprintf("%.4f", p)
}

# Exact two-sided Mann-Whitney on perv[side_a] and perv[side_b].
# Ranks are permuted; ties share the average rank.  Counts stay exact
# in a double while the repetition count stays at or below 25.
function utest_p(sa, sb,    n1, n2, N, i, j, k, v, avg, r, obs, mu, dev, key, c, s, d, total, extreme, p) {
	n1 = scount[sa]
	n2 = scount[sb]
	N = n1 + n2
	for (i = 1; i <= n1; i++) {
		val[i] = perv[sa, i]
		grp[i] = 1
	}
	for (i = 1; i <= n2; i++) {
		val[n1 + i] = perv[sb, i]
		grp[n1 + i] = 2
	}
	for (i = 2; i <= N; i++) {
		v = val[i]
		g = grp[i]
		j = i - 1
		while (j >= 1 && val[j] > v) {
			val[j + 1] = val[j]
			grp[j + 1] = grp[j]
			j--
		}
		val[j + 1] = v
		grp[j + 1] = g
	}
	i = 1
	while (i <= N) {
		j = i
		while (j < N && val[j + 1] == val[i])
			j++
		avg = (i + j) / 2
		for (k = i; k <= j; k++)
			rank2[k] = int(avg * 2 + 0.5)
		i = j + 1
	}
	obs = 0
	for (i = 1; i <= N; i++)
		if (grp[i] == 1)
			obs += rank2[i]
	for (key in dp)
		delete dp[key]
	dp[0 SUBSEP 0] = 1
	for (i = 1; i <= N; i++) {
		r = rank2[i]
		for (key in ndp)
			delete ndp[key]
		for (key in dp) {
			split(key, ks, SUBSEP)
			k = ks[1] + 0
			s = ks[2] + 0
			c = dp[key]
			ndp[k SUBSEP s] += c
			if (k < n1)
				ndp[(k + 1) SUBSEP (s + r)] += c
		}
		for (key in dp)
			delete dp[key]
		for (key in ndp)
			dp[key] = ndp[key]
	}
	mu = n1 * (N + 1)
	dev = obs - mu
	if (dev < 0)
		dev = -dev
	total = 0
	extreme = 0
	for (key in dp) {
		split(key, ks, SUBSEP)
		if (ks[1] + 0 != n1)
			continue
		s = ks[2] + 0
		c = dp[key]
		total += c
		d = s - mu
		if (d < 0)
			d = -d
		if (d >= dev)
			extreme += c
	}
	if (total == 0)
		return 1
	p = extreme / total
	if (p > 1)
		p = 1
	return p
}

function fill(side, n,    i) {
	scount[side] = n
	for (i = 1; i <= n; i++)
		perv[side, i] = xa[(side - 1) * 32 + i]
}

function selftest(    p) {
	failed = 0
	# Complete separation of 3 and 3: only the two extreme subsets.
	xa[1] = 1; xa[2] = 2; xa[3] = 3
	xa[33] = 10; xa[34] = 11; xa[35] = 12
	fill(1, 3)
	fill(2, 3)
	p = utest_p(1, 2)
	check(p, 2 / 20, "3 vs 3")

	# 4 vs 4, no overlap.  Two tails of one subset each, C(8,4) = 70.
	xa[1] = 1; xa[2] = 2; xa[3] = 3; xa[4] = 4
	xa[33] = 5; xa[34] = 6; xa[35] = 7; xa[36] = 8
	fill(1, 4)
	fill(2, 4)
	p = utest_p(1, 2)
	check(p, 2 / 70, "4 vs 4")

	# Identical samples: every assignment is the observed one.
	xa[1] = 5; xa[2] = 5; xa[3] = 5
	xa[33] = 5; xa[34] = 5; xa[35] = 5
	fill(1, 3)
	fill(2, 3)
	p = utest_p(1, 2)
	check(p, 1, "ties")
	if (failed)
		printf "selftest failed\n" > "/dev/stderr"
	else
		printf "selftest ok\n" > "/dev/stderr"
}

function check(got, want, name) {
	if (got < want - 1e-9 || got > want + 1e-9) {
		printf "FAIL %s got %.12g want %.12g\n", name, got, want > "/dev/stderr"
		failed = 1
	}
}
