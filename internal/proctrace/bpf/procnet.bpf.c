//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_endian.h>

char LICENSE[] SEC("license") = "Dual BSD/GPL";

#define COMM_LEN 16
#define AF_INET  2
#define AF_INET6 10

// total slots, the Go side has the same numbers
#define TOTAL_RX  0
#define TOTAL_TX  1
#define TOTAL_MAX 2

// a program is the name of its main thread
struct name_key {
	u8 comm[COMM_LEN];
};

// what a program received and sent since it was first seen. The counters
// only grow; the Go side reads the growth between two drains.
struct traffic_counts {
	u64 rx_bytes;
	u64 tx_bytes;
};

// per CPU, so a program busy on many CPUs does not fight over one counter.
// When the map is full the program quiet the longest makes room.
struct {
	__uint(type, BPF_MAP_TYPE_LRU_PERCPU_HASH);
	__uint(max_entries, 4096);
	__type(key, struct name_key);
	__type(value, struct traffic_counts);
} traffic SEC(".maps");

// every program together, which stays right when the map drops one
struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, TOTAL_MAX);
	__type(key, u32);
	__type(value, u64);
} totals SEC(".maps");

// loopback traffic never leaves the host. An unconnected UDP socket has no
// peer address, so what it sends and receives counts as network traffic.
static __always_inline bool loopback(struct sock *sk) {
	u16 family = BPF_CORE_READ(sk, __sk_common.skc_family);
	u32 addr[4] = {};

	if (family == AF_INET)
		return (bpf_ntohl(BPF_CORE_READ(sk, __sk_common.skc_daddr)) >> 24) == 127;
	if (family != AF_INET6 || !bpf_core_field_exists(sk->__sk_common.skc_v6_daddr))
		return false;

	BPF_CORE_READ_INTO(&addr, sk, __sk_common.skc_v6_daddr.in6_u.u6_addr32);
	if (addr[0] != 0 || addr[1] != 0)
		return false;
	// ::1, or an IPv4 loopback address mapped into IPv6
	return (addr[2] == 0 && addr[3] == bpf_htonl(1)) ||
	       (addr[2] == bpf_htonl(0xffff) && (bpf_ntohl(addr[3]) >> 24) == 127);
}

static __always_inline void add(struct sock *sk, long bytes, u32 slot) {
	struct task_struct *task;
	struct name_key key = {};
	struct traffic_counts fresh = {};
	struct traffic_counts *t;
	u64 *total;

	if (bytes <= 0 || !sk || loopback(sk))
		return;

	total = bpf_map_lookup_elem(&totals, &slot);
	if (total)
		*total += bytes;

	// the main thread's name, as threads often have names of their own
	task = (struct task_struct *)bpf_get_current_task();
	BPF_CORE_READ_STR_INTO(&key.comm, task, group_leader, comm);

	t = bpf_map_lookup_elem(&traffic, &key);
	if (!t) {
		bpf_map_update_elem(&traffic, &key, &fresh, BPF_NOEXIST);
		t = bpf_map_lookup_elem(&traffic, &key);
		if (!t)
			return;
	}
	if (slot == TOTAL_RX)
		t->rx_bytes += bytes;
	else
		t->tx_bytes += bytes;
}

// The protocols' sendmsg and recvmsg run in the calling process, and the
// kernel calls them through function pointers, so they are never inlined.
// They return the bytes actually sent or received. Their size argument
// would count a partial non-blocking write twice.

SEC("fexit/tcp_sendmsg")
int BPF_PROG(tcp_sendmsg_exit, struct sock *sk, struct msghdr *msg, size_t size, int ret) {
	add(sk, ret, TOTAL_TX);
	return 0;
}

SEC("fexit/udp_sendmsg")
int BPF_PROG(udp_sendmsg_exit, struct sock *sk, struct msghdr *msg, size_t len, int ret) {
	add(sk, ret, TOTAL_TX);
	return 0;
}

SEC("fexit/udpv6_sendmsg")
int BPF_PROG(udpv6_sendmsg_exit, struct sock *sk, struct msghdr *msg, size_t len, int ret) {
	add(sk, ret, TOTAL_TX);
	return 0;
}

// before 6.5 sendfile and splice went through tcp_sendpage, since then
// tcp_sendmsg covers them
SEC("fexit/tcp_sendpage")
int BPF_PROG(tcp_sendpage_exit, struct sock *sk, struct page *page, int offset, size_t size, int flags, int ret) {
	add(sk, ret, TOTAL_TX);
	return 0;
}

// recvmsg lost its nonblock argument in 5.19, which moved the return value.
// bpf_get_func_ret (5.17) finds it either way. Kernels without it still
// have six arguments, so the return value is the seventh slot.
static __always_inline long recv_ret(u64 *ctx) {
	u64 ret = 0;

	if (bpf_core_enum_value_exists(enum bpf_func_id, BPF_FUNC_get_func_ret)) {
		bpf_get_func_ret(ctx, &ret);
		return (int)ret;
	}
	// volatile, so the compiler cannot read it before the check above
	return (int)*(volatile u64 *)&ctx[6];
}

SEC("fexit/tcp_recvmsg")
int tcp_recvmsg_exit(u64 *ctx) {
	add((struct sock *)ctx[0], recv_ret(ctx), TOTAL_RX);
	return 0;
}

SEC("fexit/udp_recvmsg")
int udp_recvmsg_exit(u64 *ctx) {
	add((struct sock *)ctx[0], recv_ret(ctx), TOTAL_RX);
	return 0;
}

SEC("fexit/udpv6_recvmsg")
int udpv6_recvmsg_exit(u64 *ctx) {
	add((struct sock *)ctx[0], recv_ret(ctx), TOTAL_RX);
	return 0;
}
