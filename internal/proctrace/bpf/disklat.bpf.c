//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "Dual BSD/GPL";

// latency buckets, the Go side has the same number. Slot 0 is under 16 µs,
// slot k from 16·2^(k-1) µs to 16·2^k µs, the last one 2.1 s and more.
#define SLOTS 19

// the operation is the low 8 bits of cmd_flags, reads are 0 and writes 1
#define REQ_OP_MASK 0xff
#define OP_READ     0
#define OP_WRITE    1

// a whole disk's reads or writes
struct latency_key {
	u32 dev; // major << 20 | first minor, as the Go side reads it
	u32 op;
};

// how many IOs took each bucket's time. The counters only grow; the Go side
// reads the growth between two drains.
struct latency_slots {
	u64 slots[SLOTS];
};

struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_HASH);
	__uint(max_entries, 512);
	__type(key, struct latency_key);
	__type(value, struct latency_slots);
} latency SEC(".maps");

// Request flags are fixed bits up to 6.10, RQF_FLUSH_SEQ 1 << 4 and
// RQF_IO_STAT 1 << 13. Later kernels number them in enum rqf_flags.
enum rqf_flags___new {
	__RQF_FLUSH_SEQ___new = 1,
	__RQF_IO_STAT___new = 8,
};

// counted is true for the requests /proc/diskstats counts: ones with
// iostats on, and not a step of a flush sequence. A write with a cache
// flush completes once when its data is written and again when the flush
// is done, and only the second one counts.
static __always_inline bool counted(struct request *rq) {
	u32 flush_seq = 1U << 4, io_stat = 1U << 13;
	u32 flags = BPF_CORE_READ(rq, rq_flags);

	if (bpf_core_enum_value_exists(enum rqf_flags___new, __RQF_FLUSH_SEQ___new))
		flush_seq = 1U << bpf_core_enum_value(enum rqf_flags___new, __RQF_FLUSH_SEQ___new);
	if (bpf_core_enum_value_exists(enum rqf_flags___new, __RQF_IO_STAT___new))
		io_stat = 1U << bpf_core_enum_value(enum rqf_flags___new, __RQF_IO_STAT___new);
	return (flags & (flush_seq | io_stat)) == io_stat;
}

// before 5.17 a request pointed at its disk itself
struct request___old {
	struct gendisk *rq_disk;
} __attribute__((preserve_access_index));

static __always_inline struct gendisk *request_disk(struct request *rq) {
	struct request___old *old = (void *)rq;

	if (bpf_core_field_exists(old->rq_disk))
		return BPF_CORE_READ(old, rq_disk);
	return BPF_CORE_READ(rq, q, disk);
}

// floor of log2, without loops
static __always_inline u32 ilog2_32(u32 v) {
	u32 r, shift;

	r = (v > 0xffff) << 4;
	v >>= r;
	shift = (v > 0xff) << 3;
	v >>= shift;
	r |= shift;
	shift = (v > 0xf) << 2;
	v >>= shift;
	r |= shift;
	shift = (v > 0x3) << 1;
	v >>= shift;
	r |= shift;
	r |= (v >> 1);
	return r;
}

static __always_inline u32 ilog2_64(u64 v) {
	u32 hi = v >> 32;

	return hi ? ilog2_32(hi) + 32 : ilog2_32((u32)v);
}

static __always_inline u32 slot_of(u64 ns) {
	u64 us = ns / 1000;
	u32 slot;

	if (us < 16)
		return 0;
	slot = ilog2_64(us >> 4) + 1;
	return slot < SLOTS ? slot : SLOTS - 1;
}

// A request's start_time_ns is when it was made, queueing included, and
// what /proc/diskstats adds up as its read and write time.
SEC("tp_btf/block_rq_complete")
int BPF_PROG(block_rq_complete, struct request *rq, blk_status_t error, unsigned int nr_bytes) {
	struct latency_key key = {};
	struct latency_slots *counts;
	struct gendisk *disk;
	u64 start, now;
	u32 op, slot;

	// a partial completion comes back for the rest
	if (nr_bytes < BPF_CORE_READ(rq, __data_len))
		return 0;
	op = BPF_CORE_READ(rq, cmd_flags) & REQ_OP_MASK;
	if ((op != OP_READ && op != OP_WRITE) || !counted(rq))
		return 0;
	start = BPF_CORE_READ(rq, start_time_ns);
	now = bpf_ktime_get_ns();
	if (!start || now < start)
		return 0;
	disk = request_disk(rq);
	if (!disk)
		return 0;

	key.dev = (u32)BPF_CORE_READ(disk, major) << 20 | (u32)BPF_CORE_READ(disk, first_minor);
	key.op = op;
	counts = bpf_map_lookup_elem(&latency, &key);
	if (!counts) {
		struct latency_slots zero = {};

		bpf_map_update_elem(&latency, &key, &zero, BPF_NOEXIST);
		counts = bpf_map_lookup_elem(&latency, &key);
		if (!counts)
			return 0;
	}
	slot = slot_of(now - start);
	if (slot < SLOTS)
		// a completion can interrupt another on the same CPU
		__sync_fetch_and_add(&counts->slots[slot], 1);
	return 0;
}
