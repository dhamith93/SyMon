<script lang="ts">
  import { api, type AlertRule, type Channel } from '../lib/api';
  import { isAdmin } from '../lib/auth.svelte';
  import { formatAgo } from '../lib/format';
  import { describeHosts, describeLevels, describeWatch, notifies } from '../lib/rules';
  import NoChannelsNotice from '../components/NoChannelsNotice.svelte';
  import StatusBadge from '../components/StatusBadge.svelte';

  let rules = $state<AlertRule[]>([]);
  let channels = $state<Channel[]>([]);
  let loaded = $state(false);
  let error = $state('');
  // the rule being switched on or off
  let busy = $state(0);

  async function load() {
    try {
      [rules, channels] = await Promise.all([api.rules().then((r) => r.rules), api.channels().then((c) => c.channels)]);
      error = '';
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loaded = true;
    }
  }

  async function toggle(rule: AlertRule) {
    busy = rule.id;
    try {
      await api.updateRule(rule.id, !rule.enabled, rule.rule);
      await load();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = 0;
    }
  }

  load();
</script>

<div class="page">
  <header class="page-header top">
    <div>
      <h1>Alert rules</h1>
      <p class="secondary">The collector checks the rules that are on every 15 seconds, so changes apply right away.</p>
    </div>
    <div class="actions">
      <a class="control link" href="/notifications">Notification channels</a>
      {#if isAdmin()}<a class="control new" href="/rules/new">New rule</a>{/if}
    </div>
  </header>

  <NoChannelsNotice />

  {#if !isAdmin()}<p class="muted">You can see the rules. Only admins can change them.</p>{/if}

  <div class="card list">
    {#if error}
      <p class="empty">Could not load the rules: {error}</p>
    {:else if !loaded}
      <p class="empty muted">Loading…</p>
    {:else if rules.length === 0}
      <p class="empty secondary">No rules yet.</p>
    {:else}
      <div class="table-scroll">
        <table class="data">
          <caption class="sr-only">Alert rules</caption>
          <thead>
            <tr>
              <th>Rule</th>
              <th>Watches</th>
              <th>On</th>
              <th>Alerts</th>
              <th>Notifies</th>
              <th>Changed</th>
              <th class="right">Status</th>
            </tr>
          </thead>
          <tbody>
            {#each rules as rule (rule.id)}
              <tr class:off={!rule.enabled}>
                <td><a href="/rules/{rule.id}">{rule.rule.Name}</a></td>
                <td class="wrap">{describeWatch(rule.rule)}</td>
                <td class="secondary wrap">{describeHosts(rule.rule)}</td>
                <td class="secondary wrap">{describeLevels(rule.rule)}</td>
                <td class="secondary wrap">{notifies(rule.rule, channels)}</td>
                <td class="secondary">{formatAgo(rule.updatedAt)}{rule.updatedBy ? ` by ${rule.updatedBy}` : ''}</td>
                <td class="right">
                  {#if isAdmin()}
                    <button
                      class="control toggle"
                      aria-pressed={rule.enabled}
                      title={rule.enabled ? 'Switch off' : 'Switch on'}
                      disabled={busy === rule.id}
                      onclick={() => toggle(rule)}
                    >
                      {rule.enabled ? 'On' : 'Off'}
                    </button>
                  {:else}
                    <StatusBadge status={rule.enabled ? 'up' : 'down'} label={rule.enabled ? 'On' : 'Off'} />
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</div>

<style>
  .top {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    gap: 16px;
    flex-wrap: wrap;
  }

  .top p {
    margin: 4px 0 0;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .link {
    display: inline-flex;
    align-items: center;
    text-decoration: none;
    color: var(--ink);
  }

  .link:hover {
    text-decoration: none;
  }

  .new {
    display: inline-flex;
    align-items: center;
    text-decoration: none;
    color: #fff;
    background: var(--accent);
    border-color: var(--accent);
    font-weight: 600;
  }

  .new:hover {
    background: var(--accent-ink);
    text-decoration: none;
  }

  .list {
    padding: 6px 4px;
  }

  .empty {
    margin: 0;
    padding: 32px;
    text-align: center;
  }

  .wrap {
    white-space: normal;
    min-width: 160px;
  }

  tr.off td {
    opacity: 0.6;
  }

  .toggle {
    min-width: 52px;
    height: 26px;
    font-size: 12px;
  }

  .toggle[aria-pressed='true'] {
    border-color: var(--accent);
    color: var(--accent-ink);
    font-weight: 600;
  }
</style>
