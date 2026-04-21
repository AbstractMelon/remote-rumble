<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import type { ControlMode } from '$lib/stores/fight';

  const controlModeStorageKey = 'rr_control_mode';
  const dispatch = createEventDispatcher<{ ready: { index: number; controlMode: ControlMode } }>();

  let pads: Gamepad[] = [];
  let selectedIndex = -1;
  let controlMode: ControlMode = 'one-stick';
  let leftX = 0;
  let leftY = 0;
  let rightY = 0;
  let rightX = 0;
  let startPressed = false;
  let raf = 0;

  function normalizeControlMode(value: unknown): ControlMode {
    return value === 'two-stick' ? 'two-stick' : 'one-stick';
  }

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
      rightY = Number(pad.axes[3] ?? 0);
      rightX = Number(pad.axes[2] ?? 0);
      startPressed = Boolean(pad.buttons[9]?.pressed);
    } else {
      leftX = 0;
      leftY = 0;
      rightY = 0;
      startPressed = false;
    }
    raf = requestAnimationFrame(tick);
  }

  function selectPad(index: number) {
    selectedIndex = index;
  }

  function continueFlow() {
    if (selectedIndex < 0) return;
    dispatch('ready', { index: selectedIndex, controlMode });
  }

  function setControlMode(next: ControlMode) {
    controlMode = next;
    localStorage.setItem(controlModeStorageKey, next);
  }

  onMount(() => {
    controlMode = normalizeControlMode(localStorage.getItem(controlModeStorageKey));
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
  <h2>Controller setup</h2>
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
        <div class="mode-switch" role="group" aria-label="Control mode">
          <button class:active={controlMode === 'one-stick'} type="button" on:click={() => setControlMode('one-stick')}>
            One stick
          </button>
          <button class:active={controlMode === 'two-stick'} type="button" on:click={() => setControlMode('two-stick')}>
            Two stick (tank)
          </button>
        </div>
        <ul>
          {#if controlMode === 'two-stick'}
            <li>Left stick Y: left track/side</li>
            <li>Right stick Y: right track/side</li>
          {:else}
            <li>Left stick Y: throttle (forward/reverse)</li>
            <li>Left stick X: steering (left/right)</li>
          {/if}
          <li>Start button: ESC arm/disarm</li>
        </ul>

        <div class="controller-shell">
          <div class="stick-columns">
            <div class="stick-zone">
              <div class="stick-label">Left stick</div>
              <div class="stick-ring" aria-label="left stick visualizer">
                <div class="crosshair x"></div>
                <div class="crosshair y"></div>
                <div class="stick-thumb" style={`transform: translate(${(leftX * 60).toFixed(1)}px, ${(leftY * 60).toFixed(1)}px);`}></div>
              </div>
            </div>

            {#if controlMode === 'two-stick'}
              <div class="stick-zone">
                <div class="stick-label">Right stick</div>
                <div class="stick-ring" aria-label="right stick visualizer">
                  <div class="crosshair x"></div>
                  <div class="crosshair y"></div>
                  <div class="stick-thumb" style={`transform: translate(${(rightX * 60).toFixed(1)}px, ${(rightY * 60).toFixed(1)}px);`}></div>
                </div>
              </div>
            {/if}
          </div>

          <div class="axis-readout">
            {#if controlMode === 'two-stick'}
              <span>Left motor: {((leftY * -100).toFixed())}%</span>
              <span>Right motor: {((rightY * -100).toFixed())}%</span>
            {:else}
              <span>Steering: {((leftX * 100).toFixed())}%</span>
              <span>Throttle: {((leftY * -100).toFixed())}%</span>
            {/if}
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
  .mode-switch {
    display: inline-flex;
    gap: 0.4rem;
    margin-bottom: 0.7rem;
  }
  .mode-switch button {
    width: auto;
    margin: 0;
    padding: 0.35rem 0.7rem;
  }
  .mode-switch button.active {
    border-color: var(--accent);
  }
  .controller-shell {
    margin-bottom: 0.8rem;
    border: 1px solid rgba(164, 231, 233, 0.25);
    border-radius: 22px;
    background: rgba(43, 112, 122, 0.25);
    padding: 1rem;
    display: flex;
    align-items: center;
    justify-content: space-evenly;
    gap: 1.2rem;
  }
  .stick-columns {
    display: flex;
    gap: 0.8rem;
    align-items: flex-start;
  }
  .stick-zone {
    display: grid;
    gap: 0.55rem;
  }
  .stick-label {
    font-size: 0.78rem;
    color: #bddbdd;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    text-align: center;
  }
  .stick-ring {
    position: relative;
    width: 132px;
    height: 132px;
    border-radius: 999px;
    border: 2px solid rgba(167, 226, 232, 0.4);
    background: radial-gradient(circle, rgba(30, 78, 85, 0.5), rgba(7, 28, 30, 0.94));
    box-shadow: inset 0 0 24px rgba(0, 0, 0, 0.45);
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
    min-width: 129px;
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
    .stick-columns {
      width: 100%;
      justify-content: center;
      flex-wrap: wrap;
    }
  }
</style>
