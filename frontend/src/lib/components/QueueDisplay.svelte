<script lang="ts">
  import { createEventDispatcher } from 'svelte';

  export let position = 0;
  export let ahead = 0;
  export let availableBots = 0;
  export let streamUrl = '';

  const dispatch = createEventDispatcher<{ leave: void }>();
</script>

<section class="panel rise">
  <h2>Queue</h2>
  <p class="muted">You are in queue. Watch the stream while waiting for a match.</p>

  <div class="chips">
    <span class="chip ok">Position #{position || '-'}</span>
    <span class="chip">Ahead: {ahead}</span>
    <span class="chip">Available bots: {availableBots}</span>
  </div>

  <div class="stream">
    <p>You're #{position || '-'} in queue! Watch the action on the livestream while you wait.</p>
    {#if streamUrl}
      <iframe title="Livestream" src={streamUrl} allow="autoplay; fullscreen"></iframe>
    {:else}
      <p class="muted">Stream URL not yet configured.</p>
    {/if}
  </div>

  <button class="danger" on:click={() => dispatch('leave')}>Leave queue</button>
</section>

<style>
  .chips {
    display: flex;
    gap: 0.6rem;
    flex-wrap: wrap;
    margin: 0.8rem 0 1rem;
  }
  .chip {
    border-radius: 999px;
    padding: 0.24rem 0.65rem;
    background: rgba(255, 255, 255, 0.05);
  }
  .chip.ok {
    background: rgba(36, 214, 139, 0.15);
    color: #ccffe6;
  }
  .stream {
    border: 1px solid var(--line);
    border-radius: 16px;
    padding: 0.8rem;
    background: rgba(10, 29, 31, 0.65);
  }
  iframe {
    width: 100%;
    min-height: 600px;
    border: 0;
    border-radius: 12px;
    background: #0a1313;
  }
  .danger {
    margin-top: 1rem;
    background: rgba(244, 110, 81, 0.2);
    border: 1px solid rgba(244, 110, 81, 0.5);
    color: #ffd5cc;
    border-radius: 12px;
    padding: 0.6rem 0.8rem;
    cursor: pointer;
  }
  .muted {
    color: var(--muted);
  }
</style>
