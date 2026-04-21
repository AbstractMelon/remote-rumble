import { browser } from '$app/environment';
import { writable, get } from 'svelte/store';

export type SessionUser = {
  id: number;
  username: string;
  email?: string;
  isGuest: boolean;
  isAdmin: boolean;
};

type SessionState = {
  token: string;
  user: SessionUser | null;
  loading: boolean;
};

const initialToken = browser ? localStorage.getItem('rr_token') ?? '' : '';
const adminKeyStorageKey = 'rr_admin_key';

export const session = writable<SessionState>({
  token: initialToken,
  user: null,
  loading: false
});

function normalizeUser(raw: any): SessionUser {
  return {
    id: Number(raw?.id ?? 0),
    username: String(raw?.username ?? ''),
    email: raw?.email ? String(raw.email) : undefined,
    isGuest: Boolean(raw?.isGuest),
    isAdmin: Boolean(raw?.isAdmin)
  } as SessionUser;
}

export function getAdminKey() {
  if (!browser) return '';
  return localStorage.getItem(adminKeyStorageKey) ?? '';
}

export function setAdminKey(value: string) {
  if (!browser) return;
  const trimmed = value.trim();
  if (trimmed) localStorage.setItem(adminKeyStorageKey, trimmed);
  else localStorage.removeItem(adminKeyStorageKey);
}

function setToken(token: string) {
  if (!browser) return;
  if (token) localStorage.setItem('rr_token', token);
  else localStorage.removeItem('rr_token');
}

export async function apiFetch(path: string, init: RequestInit = {}) {
  const { token } = get(session);
  const headers = new Headers(init.headers ?? {});
  headers.set('Content-Type', 'application/json');
  if (token) headers.set('Authorization', `Bearer ${token}`);
  if (path.startsWith('/api/admin/')) {
    const adminKey = getAdminKey();
    if (adminKey) headers.set('X-Admin-Key', adminKey);
  }

  return fetch(path, {
    ...init,
    credentials: 'include',
    headers
  });
}

export async function verifyAdminAccess(candidateKey?: string) {
  const { token } = get(session);
  const headers = new Headers({ 'Content-Type': 'application/json' });
  const key = (candidateKey ?? getAdminKey()).trim();

  if (token) headers.set('Authorization', `Bearer ${token}`);
  if (key) headers.set('X-Admin-Key', key);

  return fetch('/api/admin/verify', {
    method: 'POST',
    credentials: 'include',
    headers,
    body: JSON.stringify({ key })
  });
}

async function runAuth(path: string, body: Record<string, string>) {
  session.update((s) => ({ ...s, loading: true }));
  try {
    const res = await apiFetch(path, { method: 'POST', body: JSON.stringify(body) });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Request failed');

    setToken(data.token);
    session.set({ token: data.token, user: normalizeUser(data.user), loading: false });
    return data;
  } finally {
    session.update((s) => ({ ...s, loading: false }));
  }
}

export function register(username: string, email: string, password: string) {
  return runAuth('/api/auth/register', { username, email, password });
}

export function login(email: string, password: string) {
  return runAuth('/api/auth/login', { email, password });
}

export function guest(username: string) {
  return runAuth('/api/auth/guest', { username });
}

export async function loadMe() {
  const { token } = get(session);
  if (!token) return;
  const res = await apiFetch('/api/auth/me');
  if (!res.ok) return;
  const data = await res.json();
  session.update((s) => ({ ...s, user: normalizeUser(data.user) }));
}

export async function logout() {
  await apiFetch('/api/auth/logout', { method: 'POST' });
  setToken('');
  session.set({ token: '', user: null, loading: false });
}
