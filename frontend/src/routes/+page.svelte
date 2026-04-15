<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import ControllerSetup from '$lib/components/ControllerSetup.svelte';
  import FightView from '$lib/components/FightView.svelte';
  import QueueDisplay from '$lib/components/QueueDisplay.svelte';
  import { appSocket } from '$lib/ws';
  import { fight, resetFlowAfterFight } from '$lib/stores/fight';
  import { queue, joinQueue, leaveQueue, applyQueueEvent } from '$lib/stores/queue';
  import { apiFetch, guest, loadMe, login, register, session, logout } from '$lib/stores/session';

  let settings = {
    stream_url: '',
    fight_duration_sec: '180'
  };

  let authTab: 'guest' | 'login' | 'register' = 'guest';
  let username = '';
  let email = '';
  let password = '';
  let authError = '';
  let selectionError = '';
  let selectedBotId = '';
  let selectedSubmitted = false;
  let timerInterval: ReturnType<typeof setInterval> | null = null;
  let socketHandlersBound = false;
  let socketUnsubs: Array<() => void> = [];

  onMount(() => {
    const boot = async () => {
      await loadMe();
      const s = get(session);
      if (s.token) connectSocket(s.token);

      const settingsRes = await fetch('/api/settings');
      if (settingsRes.ok) {
        const data = await settingsRes.json();
        settings = { ...settings, ...data };
      }

      if (s.user) {
        await restorePlayerFlow();
      }
    };

    void boot();

    timerInterval = setInterval(() => {
      fight.update((f) => {
        if (f.step !== 'fight' || !f.startedAtServer || !f.durationSec) return f;
        const elapsed = Math.floor(Date.now() / 1000) - f.startedAtServer;
        const remaining = Math.max(0, f.durationSec - elapsed);
        return { ...f, timerRemainingSec: remaining };
      });
    }, 1000);

    return () => {
      if (timerInterval) clearInterval(timerInterval);
      clearSocketHandlers();
      socketHandlersBound = false;
      appSocket.disconnect();
    };
  });

  function connectSocket(token: string) {
    appSocket.connect(token);

    if (socketHandlersBound) return;
    socketHandlersBound = true;

    socketUnsubs.push(appSocket.on('queue', (evt) => {
      const user = get(session).user;
      applyQueueEvent(evt, user?.username);
    }));

    socketUnsubs.push(appSocket.on('matched', (evt) => {
      selectedBotId = '';
      selectedSubmitted = false;
      fight.update((f) => ({
        ...f,
        step: 'selection',
        fightId: Number(evt.fightId),
        opponent: String(evt.opponent ?? '')
      }));
    }));

    socketUnsubs.push(appSocket.on('bot-list', (evt) => {
      const bots = (evt.bots ?? []) as Array<{ id: string; name: string; online: boolean; enabled: boolean }>;
      fight.update((f) => ({ ...f, bots }));
      queue.update((q) => ({ ...q, availableBots: bots.filter((b) => b.online && b.enabled).length }));
    }));

    socketUnsubs.push(appSocket.on('fight-start', (evt) => {
      selectedSubmitted = false;
      fight.update((f) => ({
        ...f,
        step: 'fight',
        fightId: Number(evt.fightId),
        botId: String(evt.botId),
        durationSec: Number(evt.durationSec ?? 180),
        startedAtServer: Number(evt.serverTime ?? Math.floor(Date.now() / 1000)),
        timerRemainingSec: Number(evt.durationSec ?? 180)
      }));
      queue.update((q) => ({ ...q, joined: false, position: 0, ahead: 0 }));
    }));

    socketUnsubs.push(appSocket.on('telemetry', (evt) => {
      fight.update((f) => ({ ...f, telemetry: evt.data ?? {} }));
    }));

    socketUnsubs.push(appSocket.on('pong', (evt) => {
      const sentAt = Number(evt.t ?? 0);
      if (!sentAt) return;
      fight.update((f) => ({ ...f, pingMs: Math.max(0, Date.now() - sentAt) }));
    }));

    socketUnsubs.push(appSocket.on('fight-end', async () => {
      resetFlowAfterFight();
      await safelyJoinQueue();
    }));
  }

  function clearSocketHandlers() {
    for (const off of socketUnsubs) off();
    socketUnsubs = [];
  }

  async function restorePlayerFlow() {
    const s = get(session);
    if (!s.user) return;

    const [queueRes, fightsRes] = await Promise.all([apiFetch('/api/queue'), apiFetch('/api/fights')]);

    if (queueRes.ok) {
      const queueData = await queueRes.json();
      applyQueueEvent(
        {
          positions: queueData.positions ?? [],
          total: Number(queueData.total ?? 0)
        },
        s.user.username
      );
    }

    if (!fightsRes.ok) {
      fight.update((f) => ({ ...f, step: 'queue' }));
      await safelyJoinQueue();
      return;
    }

    const fightsPayload = await fightsRes.json();
    const fights = (fightsPayload.fights ?? []) as Array<any>;
    const openFight = fights.find((x) => x.status === 'active' || x.status === 'selecting' || x.status === 'pending');

    if (openFight) {
      const isP1 = Number(openFight.player1Id) === s.user.id;
      const opponent = String(isP1 ? openFight.player2Name ?? '' : openFight.player1Name ?? '');

      queue.update((q) => ({ ...q, joined: false, position: 0, ahead: 0 }));

      if (openFight.status === 'active') {
        const serverTime = Number(openFight.startedAt ?? Math.floor(Date.now() / 1000));
        const duration = Number(settings.fight_duration_sec ?? '180') || 180;
        const elapsed = Math.max(0, Math.floor(Date.now() / 1000) - serverTime);
        const myBotId = String(isP1 ? openFight.bot1Id ?? '' : openFight.bot2Id ?? '');

        selectedSubmitted = false;
        fight.update((f) => ({
          ...f,
          step: 'fight',
          fightId: Number(openFight.id),
          opponent,
          botId: myBotId,
          telemetry: {},
          startedAtServer: serverTime,
          durationSec: duration,
          timerRemainingSec: Math.max(0, duration - elapsed)
        }));
        return;
      }

      selectedBotId = isP1 ? String(openFight.bot1Id ?? '') : String(openFight.bot2Id ?? '');
      selectedSubmitted = selectedBotId !== '';
      fight.update((f) => ({
        ...f,
        step: 'selection',
        fightId: Number(openFight.id),
        opponent,
        botId: '',
        telemetry: {},
        startedAtServer: 0,
        timerRemainingSec: 0
      }));
      return;
    }

    fight.update((f) => ({ ...f, step: 'queue', fightId: null, opponent: '', botId: '' }));
    if (!get(queue).joined) {
      await safelyJoinQueue();
    }
  }

  async function onControllerReady(event: CustomEvent<{ index: number }>) {
    fight.update((f) => ({
      ...f,
      controllerReady: true,
      gamepadIndex: event.detail.index,
      step: 'identity'
    }));
  }

  async function doAuth() {
    authError = '';
    try {
      if (authTab === 'guest') {
        await guest(username);
      } else if (authTab === 'login') {
        await login(email, password);
      } else {
        await register(username, email, password);
      }
      const s = get(session);
      if (s.token) connectSocket(s.token);
      await restorePlayerFlow();
    } catch (err) {
      authError = err instanceof Error ? err.message : 'Authentication failed';
    }
  }

  async function safelyJoinQueue() {
    try {
      await joinQueue();
    } catch {
      // no-op when already queued
    }
  }

  async function submitBotSelection() {
    const f = get(fight);
    selectionError = '';
    if (!f.fightId || !selectedBotId) return;

    try {
      selectedSubmitted = true;
      const res = await apiFetch(`/api/fights/${f.fightId}/bot`, {
        method: 'POST',
        body: JSON.stringify({ botId: selectedBotId })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Could not select bot');
    } catch (err) {
      selectedSubmitted = false;
      selectionError = err instanceof Error ? err.message : 'Could not select bot';
    }
  }

  async function leaveQueueNow() {
    await leaveQueue();
  }

  async function logoutNow() {
    appSocket.disconnect();
    await logout();
    queue.set({ joined: false, position: 0, ahead: 0, total: 0, availableBots: 0, positions: [] });
    fight.set({
      step: 'identity',
      controllerReady: true,
      gamepadIndex: get(fight).gamepadIndex,
      fightId: null,
      opponent: '',
      botId: '',
      bots: [],
      telemetry: {},
      pingMs: 0,
      timerRemainingSec: 0,
      startedAtServer: 0,
      durationSec: 180
    });
  }
</script>

<div class="page-wrap">
  <header class="top panel rise">
    <div>
      <h1>Remote Rumble</h1>
      <p>Remote combat robotics control platform</p>
    </div>
    {#if $session.user}
      <div class="top-right">
        <span class="chip ok">{$session.user.username}</span>
        {#if $session.user.isAdmin}
          <a class="chip" href="/admin/verify?next=/admin">Admin panel</a>
        {/if}
        <button on:click={logoutNow}>Log out</button>
      </div>
    {/if}
  </header>

  {#if $fight.step === 'controller'}
    <ControllerSetup on:ready={onControllerReady} />
  {:else if $fight.step === 'identity'}
    <section class="panel rise auth">
      <h2>Step 2: Identity</h2>
      <div class="tabs">
        <button class:active={authTab === 'guest'} on:click={() => (authTab = 'guest')}>Guest mode</button>
        <button class:active={authTab === 'login'} on:click={() => (authTab = 'login')}>Login</button>
        <button class:active={authTab === 'register'} on:click={() => (authTab = 'register')}>Signup</button>
      </div>

      {#if authTab !== 'login'}
        <label for="username">Username</label>
        <input id="username" bind:value={username} placeholder="username" />
      {/if}
      {#if authTab !== 'guest'}
        <label for="email">Email</label>
        <input id="email" bind:value={email} type="email" placeholder="you@example.com" />
        <label for="password">Password</label>
        <input id="password" bind:value={password} type="password" placeholder="password" />
      {/if}

      {#if authError}
        <p class="error">{authError}</p>
      {/if}

      <button disabled={$session.loading} on:click={doAuth}>
        {authTab === 'guest' ? 'Continue as guest' : authTab === 'login' ? 'Log in' : 'Create account'}
      </button>
    </section>
  {:else if $fight.step === 'queue'}
    <QueueDisplay
      position={$queue.position}
      ahead={$queue.ahead}
      availableBots={$queue.availableBots}
      streamUrl={settings.stream_url}
      on:leave={leaveQueueNow}
    />
  {:else if $fight.step === 'selection'}
    <section class="panel rise select">
      <h2>Step 4: Bot selection</h2>
      <p>Opponent: <strong>{$fight.opponent}</strong></p>

      <div class="bot-grid">
        {#each $fight.bots.filter((b) => b.online && b.enabled) as bot}
          <button class:selected={selectedBotId === bot.id} on:click={() => (selectedBotId = bot.id)}>
            <h3>{bot.name}</h3>
            <p>{bot.id}</p>
            <span class="chip ok">ONLINE</span>
          </button>
        {/each}
      </div>

      {#if selectionError}
        <p class="error">{selectionError}</p>
      {/if}

      <button disabled={!selectedBotId || selectedSubmitted} on:click={submitBotSelection}>
        {selectedSubmitted ? 'Waiting for opponent...' : 'Confirm bot'}
      </button>
    </section>
  {:else}
    <FightView
      botId={$fight.botId}
      gamepadIndex={$fight.gamepadIndex}
      opponent={$fight.opponent}
      telemetry={$fight.telemetry}
      pingMs={$fight.pingMs}
      timerRemainingSec={$fight.timerRemainingSec}
    />
  {/if}
</div>

<style>
  .top {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1rem;
    margin-bottom: 1rem;
  }
  h1 {
    margin: 0;
    font-size: clamp(1.4rem, 2.5vw, 2.3rem);
  }
  .top p {
    color: var(--muted);
    margin: 0.2rem 0 0;
  }
  .top-right {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .chip {
    border-radius: 999px;
    padding: 0.2rem 0.65rem;
    background: rgba(255, 255, 255, 0.07);
    color: inherit;
    text-decoration: none;
  }
  .chip.ok {
    background: rgba(36, 214, 139, 0.15);
    color: #ccffe6;
  }
  .auth {
    max-width: 560px;
    padding: 1rem;
  }
  .tabs {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }
  .tabs button.active {
    border-color: var(--accent);
  }
  label {
    display: block;
    margin: 0.5rem 0 0.2rem;
  }
  input {
    width: 100%;
    margin-bottom: 0.2rem;
  }
  .error {
    color: #ffd5cc;
  }
  .select {
    padding: 1rem;
  }
  .bot-grid {
    margin: 0.8rem 0;
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
    gap: 0.8rem;
  }
  .bot-grid button {
    text-align: left;
  }
  .bot-grid button.selected {
    border-color: var(--accent);
  }
  .bot-grid h3 {
    margin: 0;
  }
  .bot-grid p {
    margin: 0.3rem 0;
    color: var(--muted);
  }

  @media (max-width: 900px) {
    .top {
      flex-direction: column;
      align-items: flex-start;
      gap: 0.8rem;
    }
  }
</style>
