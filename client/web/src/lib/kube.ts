import type { ClusterState, ContainerUsage, KubePod, KubeWorkload } from './api';

// How a cluster's pods and workloads stand, for the Kubernetes page

export interface Badge {
  status: 'up' | 'warning' | 'critical';
  label: string;
}

// reasons that mean a pod will not get better by waiting
const failing = new Set(['CrashLoopBackOff', 'ImagePullBackOff', 'ErrImagePull', 'InvalidImageName', 'CreateContainerConfigError', 'CreateContainerError', 'OOMKilled', 'Error', 'Evicted']);

export function podStatus(pod: KubePod): Badge {
  if (pod.Phase === 'Succeeded') return { status: 'up', label: 'Completed' };
  if (pod.Phase === 'Failed') return { status: 'critical', label: pod.Reason || 'Failed' };
  if (pod.Reason) {
    const reason = pod.Reason.replace(/^Init:/, '');
    return { status: failing.has(reason) ? 'critical' : 'warning', label: pod.Reason };
  }
  if (pod.Phase === 'Pending') return { status: 'warning', label: 'Pending' };
  if (pod.Phase === 'Running') return pod.Ready ? { status: 'up', label: 'Running' } : { status: 'warning', label: 'Not ready' };
  return { status: 'warning', label: pod.Phase || 'Unknown' };
}

export function podRestarts(pod: KubePod): number {
  return (pod.Containers ?? []).reduce((total, c) => total + c.Restarts, 0);
}

// a container's run before this one ended within the last hour
export function restartedRecently(pod: KubePod, now: number): boolean {
  return (pod.Containers ?? []).some((c) => !!c.LastFinished && now - c.LastFinished < 3600);
}

// pods worth a look: not running and ready, or restarting. Finished jobs
// are fine.
export function hasProblem(pod: KubePod, now: number): boolean {
  if (pod.Phase === 'Succeeded') return false;
  return !(pod.Phase === 'Running' && pod.Ready) || restartedRecently(pod, now);
}

export function missing(w: KubeWorkload): number {
  return Math.max(w.Desired - w.Available, 0);
}

export function workloadStatus(w: KubeWorkload): Badge {
  if (w.Desired === 0) return { status: 'up', label: 'Scaled to 0' };
  const gone = missing(w);
  if (gone === 0) return { status: 'up', label: 'Ready' };
  if (w.Available === 0) return { status: 'critical', label: 'None available' };
  return { status: 'warning', label: `${gone} missing` };
}

// workloads missing replicas first, then by namespace and name
export function sortWorkloads(workloads: KubeWorkload[]): KubeWorkload[] {
  return [...workloads].sort(
    (a, b) => Number(missing(b) > 0) - Number(missing(a) > 0) || a.Namespace.localeCompare(b.Namespace) || a.Name.localeCompare(b.Name),
  );
}

export function namespaces(state: ClusterState): string[] {
  const names = new Set<string>();
  for (const w of state.Workloads ?? []) names.add(w.Namespace);
  for (const p of state.Pods ?? []) names.add(p.Namespace);
  return [...names].sort();
}

// podUsage adds up what the node's agent reports for a pod's containers,
// null when it reports none of them
export function podUsage(pod: KubePod, usage: Map<string, ContainerUsage>): { cpuPercent: number; memoryBytes: number } | null {
  let found = false;
  let cpuPercent = 0;
  let memoryBytes = 0;
  for (const c of pod.Containers ?? []) {
    const u = c.ID ? usage.get(c.ID) : undefined;
    if (!u) continue;
    found = true;
    cpuPercent += u.cpuPercent;
    memoryBytes += u.memoryBytes;
  }
  return found ? { cpuPercent, memoryBytes } : null;
}
