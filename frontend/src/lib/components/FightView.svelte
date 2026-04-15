<script lang="ts">
  import { onMount } from 'svelte';
  import TelemetryPanel from './TelemetryPanel.svelte';
  import VideoFeed from './VideoFeed.svelte';
  import { appSocket } from '$lib/ws';

  export let botId = '';
  export let gamepadIndex: number | null = null;
  export let opponent = '';
  export let telemetry = {};
  export let pingMs = 0;
  export let timerRemainingSec = 0;

  let leftX = 0;
  let leftY = 0;
  let startPressed = false;
  let raf = 0;
  let pingInterval: ReturnType<typeof setInterval> | null = null;

  function loop() {
    const pad = gamepadIndex == null ? null : (navigator.getGamepads?.()[gamepadIndex] ?? null);
    if (pad) {
      leftX = Number(pad.axes[0] ?? 0);
      leftY = Number(pad.axes[1] ?? 0);
      startPressed = Boolean(pad.buttons[9]?.pressed);

      appSocket.send({
        type: 'control',
        axes: { leftX, leftY },
        buttons: { start: startPressed },
        sentAt: Date.now()
      });
    }
    raf = requestAnimationFrame(loop);
  }

  onMount(() => {
    loop();
    pingInterval = setInterval(() => {
      appSocket.send({ type: 'ping', t: Date.now() });
    }, 1000);

    return () => {
      cancelAnimationFrame(raf);
      if (pingInterval) clearInterval(pingInterval);
    };
  });
</script>

<section class="fight rise">
  <header>
    <h2>Fight mode</h2>
    <div class="chips">
      <span class="chip">Opponent: {opponent || '-'}</span>
      <span class="chip ok">Bot: {botId || '-'}</span>
      <span class="chip warn">Time: {timerRemainingSec}s</span>
    </div>
  </header>

  <div class="layout">
    <div class="video">
      <VideoFeed />
      <div class="control-readout panel">
        <h3>Controller input</h3>
        <p>LeftX {leftX.toFixed(2)} | LeftY {leftY.toFixed(2)}</p>
        <div class="meter"><div class="fill" style={`width:${Math.min(100, Math.abs(leftX) * 100)}%`}></div></div>
        <div class="meter"><div class="fill" style={`width:${Math.min(100, Math.abs(leftY) * 100)}%`}></div></div>
        <span class={startPressed ? 'chip ok' : 'chip warn'}>{startPressed ? 'START PRESSED' : 'START RELEASED'}</span>
      </div>
    </div>
    <TelemetryPanel {telemetry} {pingMs} />
  </div>
</section>

<style>
  .fight header {
    margin-bottom: 0.8rem;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }
  .chip {
    border-radius: 999px;
    padding: 0.2rem 0.65rem;
    background: rgba(255, 255, 255, 0.06);
  }
  .chip.ok {
    background: rgba(36, 214, 139, 0.15);
    color: #ccffe6;
  }
  .chip.warn {
    background: rgba(248, 186, 61, 0.18);
    color: #ffe6b5;
  }
  .layout {
    display: grid;
    grid-template-columns: 65% 35%;
    gap: 0.9rem;
    align-items: start;
  }
  .control-readout {
    margin-top: 0.8rem;
    padding: 0.8rem;
  }
  .meter {
    height: 10px;
    margin: 0.3rem 0;
    background: #0d2426;
    border-radius: 999px;
    overflow: hidden;
  }
  .fill {
    height: 100%;
    background: linear-gradient(90deg, #ff9557, #f8ba3d);
  }

  @media (max-width: 920px) {
    .layout {
      grid-template-columns: 1fr;
    }
  }
</style>
