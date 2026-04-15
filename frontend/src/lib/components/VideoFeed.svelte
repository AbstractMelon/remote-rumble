<script lang="ts">
  import { onMount } from 'svelte';

  let state: 'idle' | 'connecting' | 'live' | 'error' = 'idle';
  let error = '';
  let video: HTMLVideoElement;
  let pc: RTCPeerConnection | null = null;

  async function connectWhep() {
    state = 'connecting';
    try {
      const configRes = await fetch('/api/stream-config');
      const config = await configRes.json();
      const whepUrl = config.whepUrl;
      if (!whepUrl) throw new Error('WHEP URL missing');

      pc = new RTCPeerConnection();
      pc.addTransceiver('video', { direction: 'recvonly' });
      pc.ontrack = (ev) => {
        const [stream] = ev.streams;
        if (video) video.srcObject = stream;
        state = 'live';
      };

      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);

      const answerRes = await fetch(whepUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/sdp' },
        body: offer.sdp
      });
      if (!answerRes.ok) throw new Error(`WHEP failed (${answerRes.status})`);

      const answerSdp = await answerRes.text();
      await pc.setRemoteDescription({ type: 'answer', sdp: answerSdp });
    } catch (err) {
      state = 'error';
      error = err instanceof Error ? err.message : 'Unable to load stream';
    }
  }

  onMount(() => {
    connectWhep();
    return () => {
      pc?.close();
      pc = null;
    };
  });
</script>

<div class="video-shell">
  <video bind:this={video} autoplay playsinline muted={false} controls={false}></video>
  {#if state === 'connecting'}
    <div class="overlay">Connecting to arena stream...</div>
  {/if}
  {#if state === 'error'}
    <div class="overlay error">{error}</div>
  {/if}
</div>

<style>
  .video-shell {
    position: relative;
    border: 1px solid var(--line);
    border-radius: 18px;
    overflow: hidden;
    min-height: 340px;
    background: #070f10;
  }
  video {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
  .overlay {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    background: rgba(0, 0, 0, 0.48);
    color: var(--ink);
    font-weight: 600;
  }
  .overlay.error {
    color: #ffd5cc;
    background: rgba(65, 9, 6, 0.6);
  }
</style>
