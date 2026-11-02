<script lang="ts">
  import { untrack } from 'svelte';
  import { align, type Aligned } from '../lib/align';
  import { api, type ClusterDetail, type ClusterSummary, type ContainerUsage } from '../lib/api';
  import { isAdmin } from '../lib/auth.svelte';
  import { appConfig } from '../lib/config.svelte';
  import { formatAgo, formatBytes, formatDateTime, formatDuration, formatNumber, formatPercent } from '../lib/format';
  import { hasProblem, missing, namespaces, podRestarts, podStatus, podUsage, sortWorkloads, workloadStatus } from '../lib/kube';
  import { poll } from '../lib/poll';
  import { hostPath, location, navigate } from '../lib/router.svelte';
  import { rangeQuery, resolveRange } from '../lib/timerange';
  import ChartCard from '../components/ChartCard.svelte';
  import StatTile from '../components/StatTile.svelte';
  import StatusBadge from '../components/StatusBadge.svelte';
  import TimeRangePicker from '../components/TimeRangePicker.svelte';

  // A Kubernetes cluster as the agent that reads it reports it, with the
  // pods' usage from the agents on its nodes. ?cluster= picks one when
  // there are several.
  type Metric = 'pods' | 'restarts';

  let now = $state(Math.floor(Date.now() / 1000));
  const range = $derived(resolveRange(location.query, now));

  let clusters = $state<ClusterSummary[]>([]);
  let detail = $state<ClusterDetail | null>(null);
  let loaded = $state(false);
  let error = $state('');
  let charts = $state<Record<Metric, { data: Aligned | null; loading: boolean; error: string }>>({
    pods: { data: null, loading: true, error: '' },
    restarts: { data: null, loading: true, error: '' },
  });
  let namespace = $state('');
  let allPods = $state(false);
  let confirmRemove = $state(false);
  let removeError = $state('');

  const picked = $derived(location.query.get('cluster') ?? '');
  const name = $derived(picked || clusters[0]?.name || '');
  const kube = $derived(detail?.state);
  const summary = $derived(detail?.summary);
  const usage = $derived(new Map<string, ContainerUsage>((detail?.usage ?? []).map((u) => [u.id, u])));
  const hosts = $derived(new Set(detail?.hosts ?? []));
  const inNamespace = (ns: string) => namespace === '' || ns === namespace;

  const nodes = $derived([...(kube?.Nodes ?? [])].sort((a, b) => Number(a.Ready) - Number(b.Ready) || a.Name.localeCompare(b.Name)));
  const workloads = $derived(sortWorkloads((kube?.Workloads ?? []).filter((w) => inNamespace(w.Namespace))));
  const pods = $derived.by(() => {
    const list = (kube?.Pods ?? []).filter((p) => inNamespace(p.Namespace) && (allPods || hasProblem(p, now)));
    // problems first, then the busiest
    const cpu = (p: (typeof list)[number]) => podUsage(p, usage)?.cpuPercent ?? -1;
    return list.sort((a, b) => Number(hasProblem(b, now)) - Number(hasProblem(a, now)) || cpu(b) - cpu(a) || a.Name.localeCompare(b.Name));
  });
  const podsOnNode = $derived.by(() => {
    const counts = new Map<string, number>();
    for (const p of kube?.Pods ?? []) if (p.Node && p.Phase === 'Running') counts.set(p.Node, (counts.get(p.Node) ?? 0) + 1);
    return counts;
  });
  const restarts = $derived((detail?.restarts ?? []).filter((r) => inNamespace(r.namespace)));
  const events = $derived((kube?.Events ?? []).filter((e) => inNamespace(e.Namespace)));

  async function loadChart(cluster: string, metric: Metric, from: number, to: number) {
    const previous = charts[metric];
    charts[metric] = { data: previous.data, loading: true, error: '' };
    try {
      const response = await api.clusterSeries(cluster, metric, from, to);
      charts[metric] = { data: align(response.series), loading: false, error: '' };
    } catch (e) {
      charts[metric] = { data: previous.data, loading: false, error: (e as Error).message };
    }
  }

  async function loadAll() {
    now = Math.floor(Date.now() / 1000);
    const { from, to } = untrack(() => range);
    try {
      clusters = (await api.clusters()).clusters;
      const cluster = untrack(() => name);
      detail = cluster ? await api.cluster(cluster) : null;
      error = '';
      if (cluster) {
        loadChart(cluster, 'pods', from, to);
        loadChart(cluster, 'restarts', from, to);
      }
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loaded = true;
    }
  }

  // reload when the cluster or range in the URL changes. Live ranges also
  // reload every refresh interval, fixed ones load once.
  $effect(() => {
    const query = location.query.toString();
    const live = resolveRange(new URLSearchParams(query)).live;
    return untrack(() => {
      if (live) return poll(loadAll, appConfig.refreshSeconds);
      loadAll();
    });
  });

  function pick(cluster: string) {
    const query = new URLSearchParams(location.query);
    query.set('cluster', cluster);
    namespace = '';
    navigate(`${location.path}?${query}`, true);
  }

  function zoom(from: number, to: number) {
    navigate(location.path + rangeQuery(location.query, { from, to }));
  }

  async function remove() {
    try {
      await api.removeCluster(name);
      appConfig.clusters = appConfig.clusters.filter((c) => c !== name);
      confirmRemove = false;
      navigate('/kubernetes', true);
    } catch (e) {
      removeError = (e as Error).message;
    }
  }

  function nodeStatus(ready: boolean, unschedulable: boolean) {
    if (!ready) return { status: 'critical' as const, label: 'Not ready' };
    return unschedulable ? { status: 'warning' as const, label: 'Ready, cordoned' } : { status: 'up' as const, label: 'Ready' };
  }
</script>

<div class="page">
  <header class="page-header">
    <h1>Kubernetes</h1>
    {#if summary}
      <p class="secondary about">
        {#if clusters.length > 1}
          <select class="control" value={name} onchange={(e) => pick(e.currentTarget.value)} aria-label="Cluster">
            {#each clusters as c (c.name)}<option>{c.name}</option>{/each}
          </select>
        {:else}
          <strong>{summary.name}</strong>
        {/if}
        <span>
          read by
          {#if summary.host}<a href={hostPath(summary.host)}>{summary.host}</a>{:else}a host that was removed{/if}, updated
          {formatAgo(summary.stateTime, now)}
        </span>
      </p>
    {/if}
  </header>

  {#if error}
    <p class="card message">Could not load clusters: {error}</p>
  {:else if loaded && clusters.length === 0}
    <div class="card message">
      <h2>No cluster reports yet</h2>
      <p class="secondary">
        Install the agent on every node, and on one control plane node give it a read-only token and
        <code>SYMON_KUBE_CLUSTER</code>, <code>SYMON_KUBE_API_URL</code>. The steps are in <code>docs/install.md</code> under Kubernetes.
      </p>
    </div>
  {/if}

  {#if summary && kube}
    {#if summary.error}
      <p class="card notice" role="alert">
        <StatusBadge status="critical" label="Cannot read the cluster" />
        <span>Since {formatAgo(summary.stateTime, now)}, the last try said: {summary.error}</span>
      </p>
    {:else if summary.stale}
      <p class="card notice" role="alert">
        <StatusBadge status="warning" label="No new state" />
        <span>Nothing since {formatDateTime(summary.stateTime)}, so alerts wait. Is the agent on {summary.host || 'its host'} running?</span>
      </p>
    {/if}

    <section class="tiles" aria-label="Cluster summary">
      <StatTile label="Nodes ready" value="{summary.nodesReady} / {summary.nodes}">
        {#snippet extra()}
          {#if summary.nodesReady < summary.nodes}<StatusBadge status="critical" label="{summary.nodes - summary.nodesReady} not ready" />{/if}
        {/snippet}
      </StatTile>
      <StatTile label="Pods running" value={summary.podsRunning} />
      <StatTile label="Pending" value={summary.podsPending} />
      <StatTile label="Not ready or failed" value={summary.podsNotReady + summary.podsFailed} />
      <StatTile label="Workloads ready" value="{summary.workloadsReady} / {summary.workloads}" />
      <StatTile label="Restarts, last hour" value={summary.restartsLastHour} />
    </section>

    <div class="filters">
      <TimeRangePicker {range} />
      <label class="secondary">
        Namespace
        <select class="control" bind:value={namespace}>
          <option value="">All</option>
          {#each namespaces(kube) as ns (ns)}<option>{ns}</option>{/each}
        </select>
      </label>
    </div>

    <div class="grid">
      <ChartCard
        title="Pods"
        unit="number"
        note="The whole cluster"
        data={charts.pods.data}
        loading={charts.pods.loading}
        error={charts.pods.error}
        syncKey="kubernetes"
        from={range.from}
        to={range.to}
        onzoom={zoom}
      />
      <ChartCard
        title="Container restarts"
        unit="number"
        note="Seen between two reports"
        data={charts.restarts.data}
        loading={charts.restarts.loading}
        error={charts.restarts.error}
        syncKey="kubernetes"
        from={range.from}
        to={range.to}
        onzoom={zoom}
      />
    </div>

    <section>
      <div class="heading">
        <h2>Pods</h2>
        <label class="check secondary">
          <input type="checkbox" bind:checked={allPods} />
          Show every pod
        </label>
      </div>
      <div class="card box">
        {#if pods.length === 0}
          <p class="muted">{allPods ? 'No pods here.' : 'Every pod is running and ready, and none restarted in the last hour.'}</p>
        {:else}
          <div class="table-scroll">
            <table class="data">
              <caption class="sr-only">Pods</caption>
              <thead>
                <tr>
                  <th>Pod</th>
                  <th>Status</th>
                  <th>Workload</th>
                  <th>Node</th>
                  <th class="right">Restarts</th>
                  <th class="right">Age</th>
                  <th class="right">CPU</th>
                  <th class="right">Memory</th>
                </tr>
              </thead>
              <tbody>
                {#each pods as pod (pod.UID)}
                  {@const badge = podStatus(pod)}
                  {@const used = podUsage(pod, usage)}
                  {@const last = (pod.Containers ?? []).find((c) => c.LastReason)}
                  <tr>
                    <td title={(pod.Containers ?? []).map((c) => `${c.Name}: ${c.Image}`).join('\n')}>
                      <span class="muted">{pod.Namespace}/</span>{pod.Name}
                    </td>
                    <td><StatusBadge status={badge.status} label={badge.label} /></td>
                    <td class="secondary">{pod.Workload}</td>
                    <td>
                      {#if pod.Node && hosts.has(pod.Node)}<a href={hostPath(pod.Node)}>{pod.Node}</a>{:else}<span class="secondary">{pod.Node || '–'}</span>{/if}
                    </td>
                    <td class="right num" title={last ? `The run before ended with ${last.LastReason}` : undefined}>
                      {podRestarts(pod)}{#if last && podRestarts(pod) > 0}<span class="muted">{` · ${last.LastReason}`}</span>{/if}
                    </td>
                    <td class="right num">{formatDuration(now - pod.Created)}</td>
                    <td class="right num">{used ? formatPercent(used.cpuPercent, 1) : '–'}</td>
                    <td class="right num">{used ? formatBytes(used.memoryBytes) : '–'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
          {#if usage.size === 0}
            <p class="muted note">CPU and memory come from the agents on the nodes, with the CRI socket readable.</p>
          {/if}
        {/if}
      </div>
    </section>

    <div class="grid tables">
      <section>
        <h2>Nodes</h2>
        <div class="card box">
          <div class="table-scroll">
            <table class="data">
              <caption class="sr-only">Nodes</caption>
              <thead>
                <tr><th>Node</th><th>Status</th><th>Roles</th><th>Version</th><th class="right">Pods</th><th class="right">CPU</th><th class="right">Memory</th></tr>
              </thead>
              <tbody>
                {#each nodes as node (node.Name)}
                  {@const badge = nodeStatus(node.Ready, node.Unschedulable)}
                  <tr>
                    <td>{#if hosts.has(node.Name)}<a href={hostPath(node.Name)}>{node.Name}</a>{:else}{node.Name}{/if}</td>
                    <td title={node.ReadyChanged ? `Since ${formatDateTime(node.ReadyChanged)}` : undefined}>
                      <StatusBadge status={badge.status} label={badge.label} />
                      {#each node.Pressure ?? [] as pressure (pressure)}<StatusBadge status="warning" label={pressure} />{/each}
                    </td>
                    <td class="secondary">{(node.Roles ?? []).join(', ') || '–'}</td>
                    <td class="secondary">{node.KubeletVersion}</td>
                    <td class="right num">{podsOnNode.get(node.Name) ?? 0}</td>
                    <td class="right num">{formatNumber(node.CPUCores)} cores</td>
                    <td class="right num">{formatBytes(node.MemoryBytes)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <section>
        <h2>Workloads</h2>
        <div class="card box">
          {#if workloads.length === 0}
            <p class="muted">No deployments, statefulsets or daemonsets here.</p>
          {:else}
            <div class="table-scroll">
              <table class="data">
                <caption class="sr-only">Workloads</caption>
                <thead><tr><th>Workload</th><th>Kind</th><th>Status</th><th class="right">Available</th></tr></thead>
                <tbody>
                  {#each workloads as w (`${w.Namespace}/${w.Kind}/${w.Name}`)}
                    {@const badge = workloadStatus(w)}
                    <tr>
                      <td><span class="muted">{w.Namespace}/</span>{w.Name}</td>
                      <td class="secondary">{w.Kind}</td>
                      <td><StatusBadge status={badge.status} label={badge.label} /></td>
                      <td class="right num" title={missing(w) > 0 ? `${w.Ready} ready` : undefined}>{w.Available} / {w.Desired}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      </section>
    </div>

    <div class="grid tables">
      <section>
        <h2>Restarts, last day</h2>
        <div class="card box">
          {#if restarts.length === 0}
            <p class="muted">No container restarted.</p>
          {:else}
            <div class="table-scroll">
              <table class="data">
                <caption class="sr-only">Container restarts in the last day</caption>
                <thead><tr><th>When</th><th>Container</th><th>Why the run ended</th><th class="right">Times</th></tr></thead>
                <tbody>
                  {#each restarts as r, i (i)}
                    <tr>
                      <td class="num">{formatDateTime(r.time)}</td>
                      <td title={r.workload}><span class="muted">{r.namespace}/</span>{r.pod}/{r.container}</td>
                      <td class="secondary">{r.reason || '–'}</td>
                      <td class="right num">{r.count}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      </section>

      <section>
        <h2>Warning events, last hour</h2>
        <div class="card box">
          {#if events.length === 0}
            <p class="muted">No warnings.</p>
          {:else}
            <div class="table-scroll">
              <table class="data">
                <caption class="sr-only">Warning events in the last hour</caption>
                <thead><tr><th>Last seen</th><th>About</th><th>Reason</th><th>Message</th><th class="right">Times</th></tr></thead>
                <tbody>
                  {#each events as e, i (i)}
                    <tr>
                      <td class="num">{formatAgo(e.LastSeen, now)}</td>
                      <td>
                        <span class="muted">{e.Namespace ? `${e.Namespace}/` : ''}</span>{e.Name}
                        <span class="muted">{e.Kind.toLowerCase()}</span>
                      </td>
                      <td class="secondary">{e.Reason}</td>
                      <td class="message-text secondary">{e.Message}</td>
                      <td class="right num">{e.Count}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      </section>
    </div>

    {#if isAdmin()}
      <section class="remove">
        {#if confirmRemove}
          <span class="secondary">Remove {name}? Its charts stay, and it comes back with the next report.</span>
          <button class="control danger" onclick={remove}>Remove</button>
          <button class="control" onclick={() => (confirmRemove = false)}>Keep</button>
        {:else}
          <span class="muted">Moving the reader to another node? Remove the cluster first, only its host may report it.</span>
          <button class="control" onclick={() => (confirmRemove = true)}>Remove cluster</button>
        {/if}
        {#if removeError}<p class="error" role="alert">{removeError}</p>{/if}
      </section>
    {/if}
  {/if}
</div>

<style>
  .about {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 10px;
    margin: 4px 0 0;
  }

  .message {
    padding: 20px;
    margin: 0 0 16px;
  }

  .message p {
    margin: 6px 0 0;
  }

  .notice {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 10px;
    padding: 12px 14px;
    margin: 0 0 16px;
    overflow-wrap: anywhere;
  }

  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 12px;
    margin-bottom: 16px;
  }

  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    padding: 10px 0;
    margin-bottom: 8px;
  }

  .filters label {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 420px), 1fr));
    gap: 12px;
    margin-bottom: 24px;
  }

  .grid.tables {
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 560px), 1fr));
    align-items: start;
    margin-bottom: 0;
  }

  section {
    margin-bottom: 24px;
    min-width: 0;
  }

  .heading {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 12px;
  }

  h2 {
    font-size: 15px;
    margin-bottom: 10px;
  }

  .check {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
  }

  .box {
    padding: 14px;
    min-width: 0;
  }

  .box p.muted {
    margin: 0;
  }

  .note {
    margin-top: 8px !important;
    font-size: 12px;
  }

  td :global(.badge) + :global(.badge) {
    margin-left: 6px;
  }

  .message-text {
    min-width: 240px;
    overflow-wrap: anywhere;
  }

  .remove {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 12px;
    font-size: 13px;
  }

  .error {
    flex-basis: 100%;
    margin: 0;
    color: var(--critical);
  }

  code {
    font-size: 12px;
    padding: 1px 4px;
    border-radius: 4px;
    background: var(--hover);
  }
</style>
