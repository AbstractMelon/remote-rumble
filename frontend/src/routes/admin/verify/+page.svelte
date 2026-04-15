<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { getAdminKey, setAdminKey, verifyAdminAccess } from '$lib/stores/session';

  let adminKey = '';
  let errorMessage = '';
  let submitting = false;

  function nextPath() {
    const next = new URLSearchParams(window.location.search).get('next') ?? '/admin';
    return next.startsWith('/admin') ? next : '/admin';
  }

  onMount(async () => {
    const existing = getAdminKey().trim();
    if (!existing) return;

    adminKey = existing;
    const res = await verifyAdminAccess(existing);
    if (res.ok) {
      goto(nextPath());
      return;
    }

    setAdminKey('');
  });

  async function submitVerify() {
    const entered = adminKey.trim();
    errorMessage = '';

    if (!entered) {
      errorMessage = 'Admin key is required';
      return;
    }

    submitting = true;
    try {
      const res = await verifyAdminAccess(entered);
      if (!res.ok) {
        let msg = 'Admin verification failed';
        try {
          const data = await res.json();
          msg = data.error ?? msg;
        } catch {
          // Keep fallback message when response body is not JSON.
        }
        setAdminKey('');
        errorMessage = msg;
        return;
      }

      setAdminKey(entered);
      goto(nextPath());
    } finally {
      submitting = false;
    }
  }
</script>

<div class="page-wrap">
  <section class="panel rise verify">
    <h1>Admin verification</h1>
    <p class="muted">Enter the admin key to unlock the admin panel.</p>

    <label for="admin_key">Admin key</label>
    <input
      id="admin_key"
      bind:value={adminKey}
      type="password"
      autocomplete="off"
      placeholder="Enter admin key"
      on:keydown={(event) => {
        if (event.key === 'Enter') submitVerify();
      }}
    />

    {#if errorMessage}
      <p class="error">{errorMessage}</p>
    {/if}

    <div class="actions">
      <button disabled={submitting} on:click={submitVerify}>
        {submitting ? 'Verifying...' : 'Verify and continue'}
      </button>
      <a href="/">Back to homepage</a>
    </div>
  </section>
</div>

<style>
  .verify {
    max-width: 580px;
    margin: 2rem auto;
    padding: 1rem;
    display: grid;
    gap: 0.65rem;
  }

  h1 {
    margin: 0;
  }

  .muted {
    color: var(--muted);
    margin: 0;
  }

  label {
    display: block;
    margin-top: 0.3rem;
  }

  input {
    width: 100%;
  }

  .error {
    color: #ffd5cc;
    margin: 0;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    flex-wrap: wrap;
  }

  a {
    color: var(--ink);
  }
</style>
