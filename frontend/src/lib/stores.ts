import { writable, derived } from 'svelte/store';
import type { UserPublic } from './api';

// Persisted auth store
function createAuthStore() {
  // Load immediately from localStorage (works client-side, safe on SSR)
  let initialToken: string | null = null;
  let initialUser: UserPublic | null = null;
  if (typeof localStorage !== 'undefined') {
    initialToken = localStorage.getItem('token');
    const userStr = localStorage.getItem('user');
    if (initialToken && userStr) {
      try { initialUser = JSON.parse(userStr) as UserPublic; } catch {
        initialToken = null;
      }
    }
  }

  const { subscribe, set, update } = writable<{
    token: string | null;
    user: UserPublic | null;
  }>({
    token: initialToken,
    user: initialUser,
  });

  return {
    subscribe,
    login: (token: string, user: UserPublic) => {
      if (typeof localStorage !== 'undefined') {
        localStorage.setItem('token', token);
        localStorage.setItem('user', JSON.stringify(user));
      }
      set({ token, user });
    },
    logout: () => {
      if (typeof localStorage !== 'undefined') {
        localStorage.removeItem('token');
        localStorage.removeItem('user');
      }
      set({ token: null, user: null });
    },
    setUser: (user: UserPublic) => {
      if (typeof localStorage !== 'undefined') {
        localStorage.setItem('user', JSON.stringify(user));
      }
      update((s) => ({ ...s, user }));
    },
    init: () => {
      if (typeof localStorage !== 'undefined') {
        const token = localStorage.getItem('token');
        const userStr = localStorage.getItem('user');
        if (token && userStr) {
          try {
            const user = JSON.parse(userStr) as UserPublic;
            set({ token, user });
          } catch {
            localStorage.removeItem('token');
            localStorage.removeItem('user');
          }
        }
      }
    },
  };
}

export const auth = createAuthStore();
export const token = derived(auth, ($auth) => $auth.token);
export const currentUser = derived(auth, ($auth) => $auth.user);
export const isAdmin = derived(auth, ($auth) => $auth.user?.role === 'admin');
export const isLoggedIn = derived(auth, ($auth) => $auth.token !== null);

// Toast notifications
export interface Toast {
  id: string;
  type: 'success' | 'error' | 'info' | 'warning';
  message: string;
  duration?: number;
}

function createToastStore() {
  const { subscribe, update } = writable<Toast[]>([]);

  function add(toast: Omit<Toast, 'id'>) {
    const id = Math.random().toString(36).slice(2);
    update((toasts) => [...toasts, { ...toast, id }]);
    setTimeout(() => remove(id), toast.duration ?? 4000);
  }

  function remove(id: string) {
    update((toasts) => toasts.filter((t) => t.id !== id));
  }

  return {
    subscribe,
    success: (message: string) => add({ type: 'success', message }),
    error: (message: string) => add({ type: 'error', message }),
    info: (message: string) => add({ type: 'info', message }),
    warning: (message: string) => add({ type: 'warning', message }),
    remove,
  };
}

export const toasts = createToastStore();
