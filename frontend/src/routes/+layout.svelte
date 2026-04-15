<script lang="ts">
  let { children } = $props();
</script>

<svelte:head>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
  <link
    href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500;600&family=Space+Grotesk:wght@400;500;600;700&display=swap"
    rel="stylesheet"
  />
  <title>Remote Rumble</title>
</svelte:head>

{@render children()}

<style>
  :global(:root) {
    --bg-0: #071719;
    --bg-1: #0f2f33;
    --ink: #ecf4f3;
    --muted: #a9c1be;
    --line: #2a4f53;
    --ok: #24d68b;
    --off: #f46e51;
    --accent: #f8ba3d;
  }

  :global(*) {
    box-sizing: border-box;
  }

  :global(body) {
    margin: 0;
    min-height: 100vh;
    color: var(--ink);
    font-family: 'Space Grotesk', system-ui, sans-serif;
    background:
      radial-gradient(1200px 700px at 9% -8%, #145d62 0%, transparent 60%),
      radial-gradient(1200px 600px at 95% -11%, rgba(244, 110, 81, 0.34) 0%, transparent 61%),
      var(--bg-0);
    position: relative;
  }

  :global(body::before) {
    content: '';
    position: fixed;
    inset: 0;
    pointer-events: none;
    background-image:
      linear-gradient(to right, rgba(255, 255, 255, 0.04) 1px, transparent 1px),
      linear-gradient(to bottom, rgba(255, 255, 255, 0.04) 1px, transparent 1px);
    background-size: 36px 36px;
    mask-image: radial-gradient(circle at center, rgba(0, 0, 0, 0.65), transparent 86%);
  }

  :global(.page-wrap) {
    max-width: 1200px;
    margin: 0 auto;
    padding: 1.2rem;
    position: relative;
    z-index: 1;
  }

  :global(.panel) {
    background: rgba(7, 25, 27, 0.7);
    padding: 10px;
    border: 1px solid #2a4f53;
    border-radius: 18px;
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
  }

  :global(input), :global(select), :global(textarea), :global(button) {
    font-family: inherit;
    border-radius: 12px;
    border: 1px solid var(--line);
    background: #0f2628;
    color: var(--ink);
    padding: 0.64rem 0.7rem;
  }

  :global(button) {
    background: #184548;
    cursor: pointer;
    transition: border-color 180ms ease, transform 180ms ease;
  }

  :global(button:hover:not(:disabled)) {
    border-color: var(--accent);
    transform: translateY(-1px);
  }

  :global(button:disabled) {
    opacity: 0.45;
    cursor: not-allowed;
  }

  :global(.rise) {
    animation: rise 380ms ease both;
  }

  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
