package proctrace

import (
	"math"
	"testing"
)

func TestBucketBounds(t *testing.T) {
	for i, want := range map[int][2]float64{
		0:  {0, 0.016},
		1:  {0.016, 0.032},
		6:  {0.512, 1.024},
		17: {1048.576, 2097.152},
		18: {2097.152, math.Inf(1)},
	} {
		if from, to := BucketBounds(i); from != want[0] || to != want[1] {
			t.Errorf("bucket %d: got %v to %v, want %v to %v", i, from, to, want[0], want[1])
		}
	}
	// every bucket starts where the one before ends
	for i := 1; i < LatencyBuckets; i++ {
		_, end := BucketBounds(i - 1)
		if start, _ := BucketBounds(i); start != end {
			t.Errorf("bucket %d starts at %v, bucket %d ends at %v", i, start, i-1, end)
		}
	}
}

func TestQuantile(t *testing.T) {
	buckets := func(set map[int]int64) []int64 {
		counts := make([]int64, LatencyBuckets)
		for i, c := range set {
			counts[i] = c
		}
		return counts
	}
	tests := []struct {
		name   string
		counts []int64
		q      float64
		want   float64
	}{
		// 100 IOs from 0.512 to 1.024 ms, the middle one half way through
		{"one bucket", buckets(map[int]int64{6: 100}), 0.5, 0.768},
		{"one bucket p99", buckets(map[int]int64{6: 100}), 0.99, 0.512 + 0.512*0.99},
		// 90 fast IOs and 10 slow ones: p50 is fast, p95 half way into the slow bucket
		{"two bands p50", buckets(map[int]int64{2: 90, 10: 10}), 0.5, 0.032 + 0.032*50.0/90},
		{"two bands p95", buckets(map[int]int64{2: 90, 10: 10}), 0.95, 8.192 + 8.192*0.5},
		{"last bucket", buckets(map[int]int64{1: 98, 18: 2}), 0.99, 2097.152},
		{"fastest", buckets(map[int]int64{0: 4}), 0.5, 0.008},
		{"q above one", buckets(map[int]int64{3: 1}), 2, 0.128},
	}
	for _, tt := range tests {
		got, ok := Quantile(tt.counts, tt.q)
		if !ok || math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("%s: got %v %v, want %v", tt.name, got, ok, tt.want)
		}
	}
	if _, ok := Quantile(make([]int64, LatencyBuckets), 0.5); ok {
		t.Error("expected no quantile without IOs")
	}
	if _, ok := Quantile(nil, 0.5); ok {
		t.Error("expected no quantile for nothing")
	}
}

func TestLatencyDelta(t *testing.T) {
	sdaRead := DiskOp{Major: 8, Minor: 0}
	sdaWrite := DiskOp{Major: 8, Minor: 0, Write: true}
	vdaRead := DiskOp{Major: 252, Minor: 0}
	nvmeRead := DiskOp{Major: 259, Minor: 0}
	prev := map[DiskOp][]uint64{
		sdaRead:  {5, 10, 0},
		sdaWrite: {1, 1, 1},
		vdaRead:  {7, 7, 7},
	}
	current := map[DiskOp][]uint64{
		sdaRead: {6, 12, 3},
		// no new writes
		sdaWrite: {1, 1, 1},
		// made again since, so it counts from zero
		vdaRead:  {2, 0, 0},
		nvmeRead: {0, 4, 0},
	}
	delta := latencyDelta(current, prev)
	want := map[DiskOp][]uint64{
		sdaRead:  {1, 2, 3},
		vdaRead:  {2, 0, 0},
		nvmeRead: {0, 4, 0},
	}
	if len(delta) != len(want) {
		t.Fatalf("got %v, want %v", delta, want)
	}
	for op, counts := range want {
		got := delta[op]
		if len(got) != len(counts) {
			t.Errorf("%+v: got %v, want %v", op, got, counts)
			continue
		}
		for i := range counts {
			if got[i] != counts[i] {
				t.Errorf("%+v: got %v, want %v", op, got, counts)
				break
			}
		}
	}
}
