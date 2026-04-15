<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';

  const dispatch = createEventDispatcher<{ ready: { index: number } }>();

  let pads: Gamepad[] = [];
  let selectedIndex = -1;
  let leftX = 0;
  let leftY = 0;
  let startPressed = false;
  let raf = 0;

  function refreshPads() {
    pads = (navigator.getGamepads?.() ?? []).filter(Boolean) as Gamepad[];
    if (selectedIndex >= 0 && !pads.find((p) => p.index === selectedIndex)) {
      selectedIndex = -1;
    }
  }

  function tick() {
    refreshPads();
    const pad = pads.find((p) => p.index === selectedIndex);
    if (pad) {
      leftX = Number(pad.axes[0] ?? 0);
      leftY = Number(pad.axes[1] ?? 0);
      startPressed = Boolean(pad.buttons[9]?.pressed);
    } else {
      leftX = 0;
      leftY = 0;
      startPressed = false;
    }
    raf = requestAnimationFrame(tick);
  }

  function selectPad(index: number) {
    selectedIndex = index;
  }

  function continueFlow() {
    if (selectedIndex < 0) return;
    dispatch('ready', { index: selectedIndex });
  }

  onMount(() => {
    window.addEventListener('gamepadconnected', refreshPads);
    window.addEventListener('gamepaddisconnected', refreshPads);
    refreshPads();
    tick();

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener('gamepadconnected', refreshPads);
      window.removeEventListener('gamepaddisconnected', refreshPads);
    };
  });
</script>

<section class="panel rise">
  <h2>Step 1: Controller setup</h2>
  <p class="muted">Connect a controller, choose it below, and confirm the live values respond.</p>

  <div class="grid two">
    <div class="card">
      <h3>Available gamepads</h3>
      {#if pads.length === 0}
        <p class="muted">No gamepads detected yet.</p>
        <p class="muted">Make sure your controller is connected and has sent input. Some browsers may require a button press to recognize the gamepad.</p>
      {:else}
        {#each pads as pad}
          <button class:selected={selectedIndex === pad.index} on:click={() => selectPad(pad.index)}>
            {pad.id || `Gamepad ${pad.index}`}
          </button>
        {/each}
      {/if}
    </div>

    <div class="card">
      <h3>Control map</h3>
      {#if selectedIndex < 0}
        <p class="muted">Waiting on gamepad selection.</p>
      {:else}
        <ul>
          <li>Left stick Y: throttle (forward/reverse)</li>
          <li>Left stick X: steering (left/right)</li>
          <li>Start button: ESC arm/disarm</li>
        </ul>
        <div class="controller-shell">
          <div class="stick-zone">
            <div class="stick-ring" aria-label="left stick visualizer">
              <div class="crosshair x"></div>
              <div class="crosshair y"></div>
              <div class="stick-thumb" style={`transform: translate(${(leftX * 60).toFixed(1)}px, ${(leftY * 60).toFixed(1)}px);`}></div>
            </div>
            <div class="axis-readout">
              <span>Steering: {((leftX * 100).toFixed())}%</span>
              <span>Throttle: {((leftY * -100).toFixed())}%</span>
            </div>
          </div>

          <div class="center-controls">
            <div class={startPressed ? 'start-button active' : 'start-button'}>
              START
            </div>
            <p class="muted small">Press and hold START to verify ESC signal.</p>
          </div>
        </div>
        <div class={startPressed ? 'chip ok' : 'chip warn'}>{startPressed ? 'START PRESSED' : 'START RELEASED'}</div>
      {/if}
    </div>
  </div>

  <button class="primary" disabled={selectedIndex < 0} on:click={continueFlow}>Looks good</button>
</section>

<style>
  .grid.two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1rem;
  }
  .card {
    border: 1px solid var(--line);
    border-radius: 16px;
    background: rgba(7, 25, 27, 0.56);
    padding: 1rem;
  }
  h2 {
    margin: 0 0 0.5rem;
  }
  h3 {
    margin: 0 0 0.7rem;
  }
  ul {
    margin: 0 0 1rem;
    padding-left: 1.1rem;
  }
  .muted {
    color: var(--muted);
  }
  button {
    width: 100%;
    text-align: left;
    margin: 0.3rem 0;
    border: 1px solid var(--line);
    background: #0f2628;
    color: var(--ink);
    border-radius: 12px;
    padding: 0.65rem;
    cursor: pointer;
  }
  button.selected {
    border-color: var(--accent);
    background: #15383c;
  }
  .controller-shell {
    margin-bottom: 0.8rem;
    border: 1px solid rgba(164, 231, 233, 0.25);
    border-radius: 22px;
    background: rgba(43, 112, 122, 0.25);
    padding: 1rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1.2rem;
  }
  .stick-zone {
    display: grid;
    gap: 0.55rem;
  }
  .stick-ring {
    position: relative;
    width: 132px;
    height: 132px;
    border-radius: 999px;
    border: 2px solid rgba(167, 226, 232, 0.4);
    background: radial-gradient(circle, rgba(30, 78, 85, 0.5), rgba(7, 28, 30, 0.94));
    box-shadow: inset 0 0 24px rgba(0, 0, 0, 0.45);
    /* overflow: hidden;  */ /* Could not decide if I wanted the stick thumb to be able to go outside the ring or not, so I left it as is for now */
  }
  .crosshair {
    position: absolute;
    background: rgba(168, 223, 227, 0.22);
  }
  .crosshair.x {
    left: 0;
    right: 0;
    top: 50%;
    height: 1px;
    transform: translateY(-50%);
  }
  .crosshair.y {
    top: 0;
    bottom: 0;
    left: 50%;
    width: 1px;
    transform: translateX(-50%);
  }
  .stick-thumb {
    position: absolute;
    width: 28px;
    height: 28px;
    border-radius: 999px;
    left: calc(50% - 14px);
    top: calc(50% - 14px);
    background: linear-gradient(180deg, #f9c85f, #f58c52);
    box-shadow: 0 0 0 3px rgba(0, 0, 0, 0.22);
    transition: transform 0.05s linear;
  }
  .axis-readout {
    display: grid;
    gap: 0.2rem;
    font-size: 0.88rem;
    color: #d6ebed;
  }
  .center-controls {
    display: grid;
    justify-items: center;
    gap: 0.45rem;
  }
  .start-button {
    width: 78px;
    height: 78px;
    border-radius: 999px;
    border: 2px solid rgba(255, 214, 146, 0.5);
    display: grid;
    place-items: center;
    font-size: 0.78rem;
    letter-spacing: 0.08em;
    color: #ffdca3;
    background: radial-gradient(circle, rgba(73, 62, 30, 0.55), rgba(18, 16, 9, 0.95));
  }
  .start-button.active {
    border-color: rgba(89, 255, 172, 0.82);
    color: #cffff0;
    box-shadow: 0 0 24px rgba(36, 214, 139, 0.36);
    background: radial-gradient(circle, rgba(21, 110, 73, 0.7), rgba(7, 31, 22, 0.96));
  }
  .small {
    font-size: 0.8rem;
    max-width: 168px;
    text-align: center;
  }
  .chip {
    display: inline-block;
    border-radius: 999px;
    padding: 0.22rem 0.65rem;
    font-size: 0.82rem;
  }
  .chip.ok {
    background: rgba(36, 214, 139, 0.15);
    color: #ccffe6;
  }
  .chip.warn {
    background: rgba(248, 186, 61, 0.18);
    color: #ffe6b5;
  }
  .primary {
    margin-top: 1rem;
    max-width: 220px;
    text-align: center;
    background: #184548;
  }

  @media (max-width: 920px) {
    .grid.two {
      grid-template-columns: 1fr;
    }
    .controller-shell {
      flex-direction: column;
      align-items: center;
    }
  }
</style>
