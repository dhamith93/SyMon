package proctrace

import "math"

// LatencyBuckets is how many latency buckets a disk histogram has, the
// same number as SLOTS in disklat.bpf.c. Bucket 0 holds IOs under 16 µs,
// bucket k those from 16·2^(k-1) µs up to 16·2^k µs, and the last one
// everything from about 2.1 seconds on.
const LatencyBuckets = 19

// BucketBounds returns where bucket i starts and ends, in ms. The last
// bucket never ends, so its end is +Inf.
func BucketBounds(i int) (from float64, to float64) {
	if i <= 0 {
		return 0, 0.016
	}
	from = 0.016 * math.Exp2(float64(i-1))
	if i >= LatencyBuckets-1 {
		return from, math.Inf(1)
	}
	return from, from * 2
}

// Quantile returns the latency in ms that a share q of the IOs in counts
// finished within, going linearly through the bucket it lands in. The last
// bucket has no end, so a quantile there reads as its start. It is false
// when counts holds no IOs.
func Quantile(counts []int64, q float64) (float64, bool) {
	var total int64
	for _, c := range counts {
		total += max(c, 0)
	}
	if total == 0 {
		return 0, false
	}
	rank := min(max(q, 0), 1) * float64(total)
	var seen float64
	last := 0
	for i, c := range counts {
		if c <= 0 {
			continue
		}
		last = i
		if seen+float64(c) >= rank {
			from, to := BucketBounds(i)
			if math.IsInf(to, 1) {
				return from, true
			}
			return from + (to-from)*(rank-seen)/float64(c), true
		}
		seen += float64(c)
	}
	// only rounding gets here
	from, _ := BucketBounds(last)
	return from, true
}

// DiskOp is one disk's reads or writes in the tracer's counts. Major and
// Minor are the whole disk's numbers, as in /proc/diskstats.
type DiskOp struct {
	Major uint32
	Minor uint32
	Write bool
}

// latencyDelta returns what each disk and op added to its buckets since
// prev. The kernel's counters only grow, so one that went down was made
// again and counts from zero. Ops with no new IOs are left out.
func latencyDelta(current map[DiskOp][]uint64, prev map[DiskOp][]uint64) map[DiskOp][]uint64 {
	delta := map[DiskOp][]uint64{}
	for op, counts := range current {
		before := prev[op]
		reset := len(before) != len(counts)
		for i := range counts {
			if !reset && counts[i] < before[i] {
				reset = true
			}
		}
		grown := make([]uint64, len(counts))
		var any bool
		for i, c := range counts {
			grown[i] = c
			if !reset {
				grown[i] -= before[i]
			}
			any = any || grown[i] > 0
		}
		if any {
			delta[op] = grown
		}
	}
	return delta
}
