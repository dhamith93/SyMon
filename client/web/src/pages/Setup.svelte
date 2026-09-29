<script lang="ts">
  import { checkSession } from '../lib/auth.svelte';
</script>

<div class="page">
  <div class="card box">
    <h1>Create the first user</h1>
    <p class="secondary">
      SyMon is locked until it has a user. On the server that runs the collector, run the following. It prints the new user's
      password.
    </p>
    <pre><code>sudo /opt/symon/collector_linux_x86_64/collector_linux_x86_64 -add-user &lt;name&gt;</code></pre>
    <button class="control" onclick={checkSession}>I have created a user</button>
  </div>
</div>

<style>
  .box {
    max-width: 560px;
    margin: 48px auto 0;
    padding: 24px;
  }

  h1 {
    margin: 0 0 8px;
  }

  p {
    margin: 0 0 12px;
  }

  pre {
    margin: 0 0 16px;
    padding: 10px 12px;
    border-radius: 6px;
    background: var(--hover);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-size: 12px;
  }
</style>
