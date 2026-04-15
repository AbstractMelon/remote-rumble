<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import { appSocket } from '$lib/ws';
  import { apiFetch, getAdminKey, session, setAdminKey, verifyAdminAccess } from '$lib/stores/session';

  let activeFight: any = null;
  let fightHistory: any[] = [];
  let bots: any[] = [];
  let queue: any[] = [];
  let users: any[] = [];
  let dbRows: any[] = [];
  let dbTable = 'users';
  let dbPage = 1;

  let stream_url = '';
  let fight_duration_sec = '180';
  let mediamtx_whep_url = 'http://localhost:8889';
  let username_blocklist = '';

  let newBot = { id: '', name: '', description: '' };
  let statusMessage = '';
  let refreshTimer: ReturnType<typeof setInterval> | null = null;
  let liveRefreshUnsubs: Array<() => void> = [];
  let refreshInFlight = false;
  let refreshQueued = false;

  async function refreshAll() {
    if (refreshInFlight) {
      refreshQueued = true;
      return;
    }

    refreshInFlight = true;
    try {
      await loadAll();
    } finally {
      refreshInFlight = false;
      if (refreshQueued) {
        refreshQueued = false;
        void refreshAll();
      }
    }
  }

  function stopLiveRefresh() {
    for (const off of liveRefreshUnsubs) off();
    liveRefreshUnsubs = [];
    if (refreshTimer) {
      clearInterval(refreshTimer);
      refreshTimer = null;
    }
  }

  function startLiveRefresh() {
    const s = get(session);
    if (s.token) {
      appSocket.connect(s.token);
    }

    const trigger = () => {
      void refreshAll();
    };

    liveRefreshUnsubs.push(appSocket.on('queue', trigger));
    liveRefreshUnsubs.push(appSocket.on('bot-list', trigger));
    liveRefreshUnsubs.push(appSocket.on('matched', trigger));
    liveRefreshUnsubs.push(appSocket.on('fight-start', trigger));
    liveRefreshUnsubs.push(appSocket.on('fight-end', trigger));

    refreshTimer = setInterval(() => {
      void refreshAll();
    }, 5000);
  }

  async function loadAll() {
    const [botsRes, queueRes, usersRes, settingsRes, fightsRes] = await Promise.all([
      apiFetch('/api/admin/bots'),
      apiFetch('/api/admin/queue'),
      apiFetch('/api/admin/users'),
      apiFetch('/api/settings'),
      apiFetch('/api/admin/fights')
    ]);

    if (!botsRes.ok || !queueRes.ok || !usersRes.ok || !fightsRes.ok) {
      throw new Error('admin authorization failed');
    }

    bots = (await botsRes.json()).bots ?? [];
    queue = (await queueRes.json()).queue ?? [];
    users = (await usersRes.json()).users ?? [];
    if (settingsRes.ok) {
      const s = await settingsRes.json();
      stream_url = s.stream_url ?? '';
      fight_duration_sec = s.fight_duration_sec ?? '180';
      mediamtx_whep_url = s.mediamtx_whep_url ?? 'http://localhost:8889';
      username_blocklist = s.username_blocklist ?? '';
    }
    fightHistory = (await fightsRes.json()).fights ?? [];
    activeFight = fightHistory.find((f) => f.status === 'active') ?? null;

    await loadDB();
  }

  async function loadDB() {
    const res = await apiFetch(`/api/admin/db/${dbTable}?page=${dbPage}`);
    dbRows = res.ok ? (await res.json()).rows ?? [] : [];
  }

  async function createMatch() {
    statusMessage = '';
    const res = await apiFetch('/api/admin/match', { method: 'POST' });
    const data = await res.json();
    if (!res.ok) {
      statusMessage = data.error || 'Failed to create match';
      return;
    }
    statusMessage = `Created fight #${data.fightId}`;
    await loadAll();
  }

  async function endFight(winnerId?: number) {
    if (!activeFight?.id) return;
    await apiFetch(`/api/admin/fights/${activeFight.id}/end`, {
      method: 'POST',
      body: JSON.stringify({ winnerId })
    });
    await loadAll();
  }

  async function addBot() {
    await apiFetch('/api/admin/bots', { method: 'POST', body: JSON.stringify(newBot) });
    newBot = { id: '', name: '', description: '' };
    await loadAll();
  }

  async function removeBot(id: string) {
    await apiFetch(`/api/admin/bots/${id}`, { method: 'DELETE' });
    await loadAll();
  }

  async function toggleBot(id: string, enabled: boolean) {
    await apiFetch(`/api/admin/bots/${id}/${enabled ? 'disable' : 'enable'}`, { method: 'POST' });
    await loadAll();
  }

  async function saveSettings() {
    await apiFetch('/api/admin/settings', {
      method: 'PUT',
      body: JSON.stringify({ stream_url, fight_duration_sec, mediamtx_whep_url, username_blocklist })
    });
    statusMessage = 'Settings updated';
  }

  function formatUnix(ts?: number) {
    if (!ts) return '-';
    return new Date(ts * 1000).toLocaleString();
  }

  function formatDuration(seconds: number) {
    const total = Math.max(0, Math.floor(seconds));
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    const secs = total % 60;

    if (hours > 0) return `${hours}h ${minutes}m`;
    if (minutes > 0) return `${minutes}m ${secs}s`;
    return `${secs}s`;
  }

  function queueWait(joinedAt?: number) {
    if (!joinedAt) return '-';
    return formatDuration(Math.floor(Date.now() / 1000) - joinedAt);
  }

  function gotoVerify() {
    goto('/admin/verify?next=/admin');
  }

  onMount(() => {
    let mounted = true;

    const boot = async () => {
      const existing = getAdminKey();
      const verifyRes = await verifyAdminAccess(existing);
      if (!verifyRes.ok) {
        setAdminKey('');
        gotoVerify();
        return;
      }

      try {
        await refreshAll();
        if (mounted) startLiveRefresh();
      } catch {
        setAdminKey('');
        gotoVerify();
      }
    };

    void boot();

    return () => {
      mounted = false;
      stopLiveRefresh();
    };
  });
</script>

<div class="page-wrap admin">
  <header class="panel rise head">
    <h1>Admin panel</h1>
    <a href="/">Back to homepage</a>
  </header>

  <section class="panel rise section fight-card">
    <h2>Fight management</h2>
    {#if activeFight}
      <p>Active fight #{activeFight.id} - {activeFight.player1Name} vs {activeFight.player2Name}</p>
      <div class="row">
        <button on:click={() => endFight()}>End fight</button>
        <button on:click={() => endFight(activeFight.player1Id)}>Winner: {activeFight.player1Name}</button>
        <button on:click={() => endFight(activeFight.player2Id)}>Winner: {activeFight.player2Name}</button>
      </div>
    {:else}
      <p>No active fight.</p>
    {/if}
    <button on:click={createMatch}>Create weighted match</button>
    <p>{statusMessage}</p>

    <h3>Fight history</h3>
    <div class="table-wrap">
      <table>
        <thead>
          <tr><th>ID</th><th>P1</th><th>P2</th><th>B1</th><th>B2</th><th>Status</th><th>Winner</th></tr>
        </thead>
        <tbody>
          {#each fightHistory as f}
            <tr>
              <td>{f.id}</td><td>{f.player1Name}</td><td>{f.player2Name}</td><td>{f.bot1Id}</td><td>{f.bot2Id}</td><td>{f.status}</td><td>{f.winnerId || '-'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </section>

  <section class="panel rise section queue-card">
    <h2>Live queue</h2>
    <div class="queue-stats">
      <span class="chip">Queued: {queue.length}</span>
      <span class="chip">Users: {users.length}</span>
      <span class="chip ok">Online bots: {bots.filter((b) => b.online && b.enabled).length}</span>
    </div>

    <div class="table-wrap">
      <table>
        <thead><tr><th>Pos</th><th>Username</th><th>Joined</th><th>Wait</th></tr></thead>
        <tbody>
          {#if queue.length === 0}
            <tr><td colspan="4">Queue is empty.</td></tr>
          {:else}
            {#each queue as q, i}
              <tr>
                <td>#{i + 1}</td>
                <td>{q.username}</td>
                <td>{formatUnix(q.joinedAt)}</td>
                <td>{queueWait(q.joinedAt)}</td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>

    <h3>Recent users</h3>
    <div class="table-wrap">
      <table>
        <thead><tr><th>User</th><th>Type</th><th>Created</th></tr></thead>
        <tbody>
          {#if users.length === 0}
            <tr><td colspan="3">No users.</td></tr>
          {:else}
            {#each users.slice(0, 10) as u}
              <tr>
                <td>{u.username}</td>
                <td>{u.isAdmin ? 'Admin' : u.isGuest ? 'Guest' : 'Registered'}</td>
                <td>{formatUnix(u.createdAt)}</td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  </section>

  <section class="panel rise section bot-card">
    <h2>Bot management</h2>
    <div class="table-wrap">
      <table>
        <thead><tr><th>Name</th><th>ID</th><th>Online</th><th>Enabled</th><th>Actions</th></tr></thead>
        <tbody>
          {#each bots as b}
            <tr>
              <td>{b.name}</td>
              <td>{b.id}</td>
              <td><span class={b.online ? 'chip ok' : 'chip err'}>{b.online ? 'ONLINE' : 'OFFLINE'}</span></td>
              <td>{b.enabled ? 'Yes' : 'No'}</td>
              <td>
                <button on:click={() => toggleBot(b.id, b.enabled)}>{b.enabled ? 'Disable' : 'Enable'}</button>
                <button on:click={() => removeBot(b.id)}>Remove</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <h3>Add bot</h3>
    <div class="form-grid">
      <input bind:value={newBot.id} placeholder="bot id" />
      <input bind:value={newBot.name} placeholder="bot name" />
      <input bind:value={newBot.description} placeholder="description" />
      <button on:click={addBot}>Add bot</button>
    </div>
  </section>

  <section class="panel rise section db-card">
    <h2>Database viewer</h2>
    <div class="row">
      <select bind:value={dbTable}>
        <option value="users">users</option>
        <option value="fights">fights</option>
        <option value="bots">bots</option>
        <option value="queue">queue</option>
      </select>
      <button on:click={() => { dbPage = 1; loadDB(); }}>Load</button>
      <button on:click={() => { dbPage = Math.max(1, dbPage - 1); loadDB(); }}>Prev</button>
      <button on:click={() => { dbPage += 1; loadDB(); }}>Next</button>
      <span>Page {dbPage}</span>
    </div>

    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            {#if dbRows[0]}
              {#each Object.keys(dbRows[0]) as key}<th>{key}</th>{/each}
            {/if}
          </tr>
        </thead>
        <tbody>
          {#each dbRows as row}
            <tr>
              {#each Object.values(row) as value}<td>{String(value ?? '')}</td>{/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </section>

  <section class="panel rise section settings-card">
    <h2>Settings</h2>
    <label for="stream_url">Livestream URL</label>
    <input id="stream_url" bind:value={stream_url} />
    <label for="fight_duration_sec">Fight duration (seconds)</label>
    <input id="fight_duration_sec" bind:value={fight_duration_sec} type="number" min="30" />
    <label for="mediamtx_whep_url">mediamtx WHEP base URL</label>
    <input id="mediamtx_whep_url" bind:value={mediamtx_whep_url} />
    <label for="username_blocklist">Username blocklist (one per line)</label>
    <textarea id="username_blocklist" rows="6" bind:value={username_blocklist}></textarea>
    <button on:click={saveSettings}>Save settings</button>
  </section>
</div>

<style>
  .head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1rem;
    margin-bottom: 1rem;
  }
  .admin {
    display: grid;
    grid-template-columns: repeat(12, minmax(0, 1fr));
    gap: 1rem;
  }
  .head {
    grid-column: 1 / -1;
  }
  .section {
    padding: 1rem;
  }
  .fight-card {
    grid-column: span 8;
  }
  .queue-card {
    grid-column: span 4;
  }
  .bot-card {
    grid-column: span 7;
  }
  .settings-card {
    grid-column: span 5;
  }
  .db-card {
    grid-column: 1 / -1;
  }
  .table-wrap {
    overflow-x: auto;
    border: 1px solid var(--line);
    border-radius: 12px;
  }
  table {
    width: 100%;
    border-collapse: collapse;
  }
  th,
  td {
    text-align: left;
    padding: 0.45rem;
    border-bottom: 1px solid rgba(255, 255, 255, 0.08);
    font-size: 0.88rem;
  }
  .row {
    display: flex;
    gap: 0.45rem;
    flex-wrap: wrap;
    align-items: center;
    margin-bottom: 0.6rem;
  }
  .queue-stats {
    display: flex;
    gap: 0.45rem;
    flex-wrap: wrap;
    margin-bottom: 0.65rem;
  }
  .chip {
    border-radius: 999px;
    padding: 0.2rem 0.6rem;
  }
  .chip.ok {
    background: rgba(36, 214, 139, 0.15);
    color: #ccffe6;
  }
  .chip.err {
    background: rgba(244, 110, 81, 0.16);
    color: #ffd5cc;
  }
  .form-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 0.5rem;
    align-items: end;
  }
  input,
  textarea,
  select {
    width: 100%;
  }
  label {
    display: block;
    margin: 0.5rem 0 0.2rem;
  }

  @media (max-width: 1180px) {
    .fight-card,
    .queue-card,
    .bot-card,
    .settings-card,
    .db-card {
      grid-column: 1 / -1;
    }
  }
</style>
