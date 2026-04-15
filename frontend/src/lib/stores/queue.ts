import { writable } from 'svelte/store';
import { apiFetch } from './session';

export type QueueState = {
  joined: boolean;
  position: number;
  ahead: number;
  total: number;
  availableBots: number;
  positions: Array<{ username: string; pos: number }>;
};

export const queue = writable<QueueState>({
  joined: false,
  position: 0,
  ahead: 0,
  total: 0,
  availableBots: 0,
  positions: []
});

export async function joinQueue() {
  const res = await apiFetch('/api/queue/join', { method: 'POST' });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'Could not join queue');

  queue.update((q) => ({
    ...q,
    joined: true,
    position: data.position,
    ahead: data.ahead,
    total: data.total,
    availableBots: data.availableBots
  }));
}

export async function leaveQueue() {
  await apiFetch('/api/queue/leave', { method: 'DELETE' });
  queue.update((q) => ({ ...q, joined: false, position: 0, ahead: 0 }));
}

export function applyQueueEvent(event: { positions: Array<{ username: string; pos: number }>; total: number }, username?: string) {
  const position = username ? event.positions.find((x) => x.username.toLowerCase() === username.toLowerCase())?.pos ?? 0 : 0;
  queue.update((q) => ({
    ...q,
    positions: event.positions,
    total: event.total,
    position,
    ahead: position > 0 ? position - 1 : 0,
    joined: position > 0
  }));
}
