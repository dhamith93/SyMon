//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>

char LICENSE[] SEC("license") = "Dual BSD/GPL";

#define COMM_LEN 16

// counter slots, the Go side has the same numbers
#define CNT_EXECS        0
#define CNT_FAILED_EXECS 1
#define CNT_EXITS        2
#define CNT_NONZERO      3
#define CNT_SIGNALED     4
#define CNT_DROPS        5
#define CNT_MAX          6

// exit_event is a process that ended with a non-zero status or a signal
struct exit_event {
	u64 lifetime_ns;
	u32 pid;
	u32 ppid;
	u32 uid;
	s32 exit_code;           // status << 8 | core dumped (0x80) | signal
	u8  comm[COMM_LEN];
	u8  parent_comm[COMM_LEN];
	u8  oom;                 // the OOM killer picked it
	u8  _pad[7];
};

// per CPU, so the hot path needs no atomics. The Go side adds the CPUs up.
struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, CNT_MAX);
	__type(key, u32);
	__type(value, u64);
} counters SEC(".maps");

// processes the OOM killer chose, until they exit
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 1024);
	__type(key, u32);
	__type(value, u8);
} oom_victims SEC(".maps");

// the Go side sets the size before loading
struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 256 * 1024);
} events SEC(".maps");

const struct exit_event *unused_exit_event __attribute__((unused));

static __always_inline void count(u32 slot) {
	u64 *value = bpf_map_lookup_elem(&counters, &slot);
	if (value)
		(*value)++;
}

static __always_inline void count_exec(long ret) {
	count(ret == 0 ? CNT_EXECS : CNT_FAILED_EXECS);
}

// a failed exec only shows on the way out, so the exit side is enough here

SEC("tracepoint/syscalls/sys_exit_execve")
int exit_execve(struct trace_event_raw_sys_exit *ctx) {
	count_exec(ctx->ret);
	return 0;
}

SEC("tracepoint/syscalls/sys_exit_execveat")
int exit_execveat(struct trace_event_raw_sys_exit *ctx) {
	count_exec(ctx->ret);
	return 0;
}

SEC("tracepoint/oom/mark_victim")
int mark_victim(struct trace_event_raw_mark_victim *ctx) {
	u32 pid = ctx->pid;
	u8 yes = 1;

	bpf_map_update_elem(&oom_victims, &pid, &yes, BPF_ANY);
	return 0;
}

SEC("tracepoint/sched/sched_process_exit")
int process_exit(void *ctx) {
	struct task_struct *task = (struct task_struct *)bpf_get_current_task();
	struct exit_event *e;
	u32 pid;
	s32 code;
	u8 *oom;

	// this fires for every thread. The process is gone when its last
	// thread leaves, and the kernel has already counted that one down.
	if (BPF_CORE_READ(task, signal, live.counter) != 0)
		return 0;

	pid = bpf_get_current_pid_tgid() >> 32;
	code = BPF_CORE_READ(task, exit_code);
	count(CNT_EXITS);

	oom = bpf_map_lookup_elem(&oom_victims, &pid);

	if (code & 0x7f)
		count(CNT_SIGNALED);
	else if ((code >> 8) & 0xff)
		count(CNT_NONZERO);
	else if (!oom)
		return 0; // a clean exit, only counted

	e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
	if (!e) {
		count(CNT_DROPS);
		if (oom)
			bpf_map_delete_elem(&oom_victims, &pid);
		return 0;
	}

	e->pid = pid;
	e->ppid = BPF_CORE_READ(task, real_parent, tgid);
	e->uid = bpf_get_current_uid_gid() & 0xffffffff;
	e->exit_code = code;
	// the leader's start and name, since the last thread out may be any thread
	e->lifetime_ns = bpf_ktime_get_ns() - BPF_CORE_READ(task, group_leader, start_time);
	BPF_CORE_READ_STR_INTO(&e->comm, task, group_leader, comm);
	BPF_CORE_READ_STR_INTO(&e->parent_comm, task, real_parent, comm);
	e->oom = 0;
	if (oom) {
		e->oom = 1;
		bpf_map_delete_elem(&oom_victims, &pid);
	}

	bpf_ringbuf_submit(e, 0);
	return 0;
}