import { describe, expect, it } from 'vitest';
import type { ClusterState, ContainerUsage, KubePod, KubeWorkload } from './api';
import { hasProblem, namespaces, podRestarts, podStatus, podUsage, sortWorkloads, workloadStatus } from './kube';

const now = 1700000000;

const pod = (fields: Partial<KubePod>): KubePod => ({
  Namespace: 'media',
  Name: 'web-1',
  UID: 'u1',
  Workload: 'deployment/web',
  Phase: 'Running',
  Ready: true,
  Created: now - 3600,
  Containers: [],
  ...fields,
});

const workload = (fields: Partial<KubeWorkload>): KubeWorkload => ({ Namespace: 'media', Kind: 'deployment', Name: 'web', Desired: 2, Ready: 2, Available: 2, ...fields });

describe('pods', () => {
  it('says how a pod stands', () => {
    expect(podStatus(pod({}))).toEqual({ status: 'up', label: 'Running' });
    expect(podStatus(pod({ Ready: false }))).toEqual({ status: 'warning', label: 'Not ready' });
    expect(podStatus(pod({ Ready: false, Reason: 'CrashLoopBackOff' }))).toEqual({ status: 'critical', label: 'CrashLoopBackOff' });
    expect(podStatus(pod({ Phase: 'Pending', Ready: false, Reason: 'Init:ImagePullBackOff' }))).toEqual({ status: 'critical', label: 'Init:ImagePullBackOff' });
    expect(podStatus(pod({ Phase: 'Pending', Ready: false, Reason: 'Unschedulable' }))).toEqual({ status: 'warning', label: 'Unschedulable' });
    expect(podStatus(pod({ Phase: 'Pending', Ready: false }))).toEqual({ status: 'warning', label: 'Pending' });
    expect(podStatus(pod({ Phase: 'Succeeded', Ready: false }))).toEqual({ status: 'up', label: 'Completed' });
    expect(podStatus(pod({ Phase: 'Failed', Ready: false, Reason: 'Evicted' }))).toEqual({ status: 'critical', label: 'Evicted' });
  });

  it('finds the pods worth a look', () => {
    const restarted = pod({ Containers: [{ Name: 'web', Image: 'nginx', Ready: true, Restarts: 3, State: 'running', LastReason: 'OOMKilled', LastFinished: now - 600 }] });
    expect(hasProblem(pod({}), now)).toBe(false);
    expect(hasProblem(restarted, now)).toBe(true);
    expect(hasProblem(restarted, now + 7200)).toBe(false);
    expect(hasProblem(pod({ Phase: 'Pending', Ready: false }), now)).toBe(true);
    expect(hasProblem(pod({ Phase: 'Succeeded', Ready: false }), now)).toBe(false);
    expect(podRestarts(restarted)).toBe(3);
    expect(podRestarts(pod({ Containers: null }))).toBe(0);
  });

  it('adds up what the node reports for a pod', () => {
    const usage = new Map<string, ContainerUsage>([
      ['a', { id: 'a', host: 'w1', cpuPercent: 10, cpuCores: 0.4, memoryBytes: 100, memoryLimit: 0 }],
      ['b', { id: 'b', host: 'w1', cpuPercent: 2.5, cpuCores: 0.1, memoryBytes: 50, memoryLimit: 0 }],
    ]);
    const two = pod({
      Containers: [
        { Name: 'web', ID: 'a', Image: 'nginx', Ready: true, Restarts: 0, State: 'running' },
        { Name: 'sidecar', ID: 'b', Image: 'envoy', Ready: true, Restarts: 0, State: 'running' },
      ],
    });
    expect(podUsage(two, usage)).toEqual({ cpuPercent: 12.5, memoryBytes: 150 });
    expect(podUsage(pod({ Containers: [{ Name: 'x', ID: 'z', Image: '', Ready: true, Restarts: 0, State: 'running' }] }), usage)).toBeNull();
  });
});

describe('workloads', () => {
  it('says how a workload stands', () => {
    expect(workloadStatus(workload({}))).toEqual({ status: 'up', label: 'Ready' });
    expect(workloadStatus(workload({ Available: 1 }))).toEqual({ status: 'warning', label: '1 missing' });
    expect(workloadStatus(workload({ Available: 0 }))).toEqual({ status: 'critical', label: 'None available' });
    expect(workloadStatus(workload({ Desired: 0, Available: 0 }))).toEqual({ status: 'up', label: 'Scaled to 0' });
  });

  it('puts the ones missing replicas first', () => {
    const sorted = sortWorkloads([workload({ Namespace: 'b', Name: 'x' }), workload({ Namespace: 'a', Name: 'y' }), workload({ Namespace: 'c', Name: 'z', Available: 1 })]);
    expect(sorted.map((w) => w.Name)).toEqual(['z', 'y', 'x']);
  });

  it('lists the namespaces', () => {
    const state: ClusterState = { Cluster: 'homelab', Time: now, Nodes: null, Workloads: [workload({ Namespace: 'media' })], Pods: [pod({ Namespace: 'default' })], Events: null };
    expect(namespaces(state)).toEqual(['default', 'media']);
  });
});
