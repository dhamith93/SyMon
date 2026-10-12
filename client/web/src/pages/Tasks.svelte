<script lang="ts">
  import { untrack } from 'svelte';
  import { api, type HostTask, type TaskRun } from '../lib/api';
  import { isAdmin } from '../lib/auth.svelte';
  import { formatAgo, formatDateTime, formatDuration } from '../lib/format';
  import { poll } from '../lib/poll';
  import { hostPath } from '../lib/router.svelte';
  import { describeRun, isActive, lastRuns, runDuration } from '../lib/tasks';
  import StatusBadge from '../components/StatusBadge.svelte';

  // The commands a host lists in its own tasks file. Admins start them
  // here, and everyone sees how the runs went.
  let { host }: { host: string } = $props();

  let tasks = $state<HostTask[]>([]);
  let runs = $state<TaskRun[]>([]);
  let up = $state(false);
  let loaded = $state(false);
  let error = $state('');
  let now = $state(Date.now() / 1000);

  // the task waiting for a second click, and the run being asked for
  let confirming = $state('');
  let starting = $state(false);
  let runError = $state('');

  // the run whose output is open, with what it has printed so far
  let open = $state(0);
  let openRun = $state<TaskRun | null>(null);
  let openError = $state('');

  const last = $derived(lastRuns(runs));
  const active = $derived(runs.find(isActive));

  async function load() {
    try {
      const [taskList, runList, fleet] = await Promise.all([api.hostTasks(host), api.taskRuns(host), api.fleet()]);
      tasks = taskList.tasks;
      runs = runList.runs;
      up = fleet.hosts.find((h) => h.name === host)?.up ?? false;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    } finally {
      now = Date.now() / 1000;
      loaded = true;
    }
  }

  $effect(() => {
    void host;
    return untrack(() => poll(load, 5));
  });

  async function start(task: HostTask) {
    starting = true;
    runError = '';
    try {
      const run = await api.runTask(host, task.name);
      confirming = '';
      open = run.id;
      await load();
    } catch (e) {
      runError = (e as Error).message;
    } finally {
      starting = false;
    }
  }

  function toggle(run: TaskRun) {
    open = open === run.id ? 0 : run.id;
  }

  // an open run is read again every 2 seconds for as long as it goes, so
  // its output shows up while it runs
  $effect(() => {
    const id = open;
    openRun = null;
    openError = '';
    if (!id) return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    async function read() {
      try {
        const run = await api.taskRun(id);
        if (stopped) return;
        openRun = run;
        openError = '';
        if (isActive(run)) {
          timer = setTimeout(read, 2000);
        } else {
          // the lists learn how it ended right away
          load();
        }
      } catch (e) {
        if (!stopped) openError = (e as Error).message;
      }
    }
    untrack(read);
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  });

  // why a task cannot be started right now, empty when it can
  const blocked = $derived(!up ? 'This host is not reporting' : active ? 'This host is running a task' : '');
</script>

{#snippet result(run: TaskRun)}
  {@const state = describeRun(run)}
  {#if state.tone === 'busy'}
    <span class="busy"><span class="dot" aria-hidden="true"></span>{state.label}</span>
  {:else}
    <StatusBadge status={state.tone === 'good' ? 'up' : state.tone === 'bad' ? 'critical' : 'warning'} label={state.label} />
  {/if}
{/snippet}

<div class="page">
  <header class="page-header">
    <p class="crumbs muted"><a href="/">Hosts</a> / <a href={hostPath(host)}>{host}</a> /</p>
    <h1>Tasks</h1>
    <p class="secondary intro">
      Commands this host lists in its own tasks file. Nothing else can be run from here.
      {#if !isAdmin()}Only admins can run them.{/if}
    </p>
  </header>

  {#if error}
    <div class="card empty"><p>Could not load the tasks: {error}</p></div>
  {:else if !loaded}
    <div class="card empty"><p class="muted">Loading…</p></div>
  {:else}
    <section>
      {#if tasks.length === 0}
        <div class="card empty">
          <p>This host offers no tasks.</p>
          <p class="secondary">
            List them in <code>/etc/symon/tasks.json</code> on the host. They show up here within a minute, without restarting the agent.
          </p>
          <pre class="sample">{`[
  { "Name": "Restart nginx", "Command": "systemctl restart nginx" }
]`}</pre>
          <p class="secondary">The file has to belong to root and must not be writable by others. Agents from before tasks need an update.</p>
        </div>
      {:else}
        {#if runError}<p class="error" role="alert">Could not run the task: {runError}</p>{/if}
        <div class="card list">
          <div class="table-scroll">
            <table class="data">
              <caption class="sr-only">Tasks of {host}</caption>
              <thead>
                <tr>
                  <th>Task</th>
                  {#if isAdmin()}<th>Command</th>{/if}
                  <th>Time limit</th>
                  <th>Last run</th>
                  {#if isAdmin()}<th class="right">Run</th>{/if}
                </tr>
              </thead>
              <tbody>
                {#each tasks as task (task.name)}
                  {@const lastRun = last.get(task.name)}
                  <tr>
                    <td class="wrap">
                      <strong>{task.name}</strong>
                      {#if task.description}<div class="secondary">{task.description}</div>{/if}
                    </td>
                    {#if isAdmin()}<td class="wrap"><code>{task.command}</code></td>{/if}
                    <td class="secondary num">{formatDuration(task.timeoutSeconds)}</td>
                    <td>
                      {#if lastRun}
                        {@render result(lastRun)}
                        <span class="muted when" title={formatDateTime(lastRun.requestedAt)}>{formatAgo(lastRun.requestedAt, now)}</span>
                      {:else}
                        <span class="muted">Never</span>
                      {/if}
                    </td>
                    {#if isAdmin()}
                      <td class="right">
                        {#if confirming === task.name}
                          <span class="secondary">Run it on {host}?</span>
                          <button class="control primary" disabled={starting || !!blocked} onclick={() => start(task)}>Run</button>
                          <button class="control" disabled={starting} onclick={() => (confirming = '')}>Cancel</button>
                        {:else}
                          <button
                            class="control"
                            disabled={!!blocked}
                            title={blocked}
                            onclick={() => {
                              confirming = task.name;
                              runError = '';
                            }}
                          >
                            Run
                          </button>
                        {/if}
                      </td>
                    {/if}
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>
        {#if isAdmin() && blocked}<p class="muted note">{blocked}, so tasks cannot be started right now.</p>{/if}
      {/if}
    </section>

    <section>
      <h2>History</h2>
      <div class="card list">
        {#if runs.length === 0}
          <p class="none secondary">No task has been run on this host yet.</p>
        {:else}
          <div class="table-scroll">
            <table class="data">
              <caption class="sr-only">Task runs on {host}, newest first</caption>
              <thead>
                <tr>
                  <th>Asked</th>
                  <th>Task</th>
                  <th>By</th>
                  <th>Result</th>
                  <th>Took</th>
                  {#if isAdmin()}<th class="right">Output</th>{/if}
                </tr>
              </thead>
              <tbody>
                {#each runs as run (run.id)}
                  <tr class:opened={open === run.id}>
                    <td class="num" title={formatAgo(run.requestedAt, now)}>{formatDateTime(run.requestedAt)}</td>
                    <td>{run.task}</td>
                    <td class="secondary">{run.requestedBy}</td>
                    <td class="wrap">
                      {@render result(run)}
                      {#if run.error}<div class="secondary">{run.error}</div>{/if}
                    </td>
                    <td class="secondary num">{runDuration(run, now)}</td>
                    {#if isAdmin()}
                      <td class="right">
                        <button class="control small" aria-expanded={open === run.id} onclick={() => toggle(run)}>
                          {open === run.id ? 'Hide' : 'Show'}
                        </button>
                      </td>
                    {/if}
                  </tr>
                  {#if isAdmin() && open === run.id}
                    <tr class="detail">
                      <td colspan="6">
                        <p class="secondary ran">Ran <code>{run.command}</code></p>
                        {#if openError}
                          <p class="error">Could not load the output: {openError}</p>
                        {:else if !openRun}
                          <p class="muted">Loading…</p>
                        {:else if openRun.output}
                          <pre class="output">{openRun.output}</pre>
                          {#if openRun.truncated}<p class="muted">The output was cut off at 256 KB. The task itself carried on.</p>{/if}
                        {:else if isActive(openRun)}
                          <p class="muted">Nothing printed yet.</p>
                        {:else}
                          <p class="muted">The task printed nothing.</p>
                        {/if}
                      </td>
                    </tr>
                  {/if}
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    </section>
  {/if}
</div>

<style>
  .crumbs {
    margin: 0 0 2px;
    font-size: 12px;
  }

  .intro {
    margin: 4px 0 0;
  }

  section {
    margin-bottom: 24px;
  }

  h2 {
    font-size: 15px;
    margin-bottom: 10px;
  }

  .list {
    padding: 6px 4px;
  }

  .empty {
    padding: 20px;
  }

  .empty p {
    margin: 0 0 6px;
  }

  .none {
    margin: 0;
    padding: 24px;
    text-align: center;
  }

  .note {
    margin: 8px 2px 0;
    font-size: 12px;
  }

  .wrap {
    white-space: normal;
    min-width: 180px;
  }

  .when {
    margin-left: 6px;
    font-size: 12px;
  }

  /* a run that is waiting or going: no status color, it is not a verdict */
  .busy {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    font-weight: 500;
    color: var(--ink-secondary);
    white-space: nowrap;
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--accent);
    animation: pulse 1.2s ease-in-out infinite;
  }

  @keyframes pulse {
    50% {
      opacity: 0.3;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .dot {
      animation: none;
    }
  }

  .primary {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
    font-weight: 600;
  }

  .primary:hover {
    background: var(--accent-ink);
  }

  .control:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .small {
    height: 26px;
    font-size: 12px;
  }

  td.right {
    white-space: normal;
  }

  td.right .secondary {
    margin-right: 6px;
    font-size: 12px;
  }

  .error {
    margin: 0 0 10px;
    font-size: 13px;
    color: var(--critical-ink);
  }

  tr.opened td {
    border-bottom-color: transparent;
  }

  tr.detail:hover {
    background: none;
  }

  tr.detail td {
    padding: 4px 10px 14px;
    white-space: normal;
  }

  tr.detail p {
    margin: 0 0 8px;
    font-size: 12px;
  }

  code,
  pre {
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 12px;
  }

  code {
    padding: 1px 4px;
    border-radius: 4px;
    background: var(--hover);
    overflow-wrap: anywhere;
  }

  pre {
    margin: 0 0 8px;
    padding: 10px 12px;
    border-radius: 6px;
    background: var(--hover);
    overflow: auto;
  }

  .output {
    max-height: 420px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .sample {
    display: inline-block;
    min-width: min(100%, 420px);
  }
</style>
