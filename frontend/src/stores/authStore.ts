import { create } from 'zustand';
import { getNanoClient, resetNanoClient } from '@/services/nanoClient';
import { toast } from 'sonner';

interface Player {
  player_id: number;
  username: string;
}

interface AuthState {
  player: Player | null;
  deviceId: string;
  isLoggedIn: boolean;
  isConnecting: boolean;
  isAuthChecked: boolean;
  login: (username: string) => Promise<void>;
  checkSession: () => Promise<void>;
  logout: () => void;
}

function generateId(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  // Fallback for older browsers
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
  });
}

function getOrCreateDeviceId(): string {
  let id = localStorage.getItem('cg_device_id');
  if (!id) {
    id = generateId();
    localStorage.setItem('cg_device_id', id);
  }
  return id;
}

let reconnectHandler: (() => void) | null = null;

export const useAuthStore = create<AuthState>((set, get) => ({
  player: null,
  deviceId: getOrCreateDeviceId(),
  isLoggedIn: false,
  isConnecting: false,
  isAuthChecked: false,

  login: async (username: string) => {
    if (get().isConnecting) return;
    set({ isConnecting: true });
    try {
      const client = getNanoClient();
      if (!client.connected) {
        await client.connect();
      }
      // Remove old reconnect handler before adding new one
      if (reconnectHandler) {
        client.off('reconnect', reconnectHandler);
      }
      reconnectHandler = () => {
        const saved = localStorage.getItem('cg_username');
        if (saved) {
          get().checkSession();
        }
      };
      client.on('reconnect', reconnectHandler);

      const res = (await client.request('GuestAuth.GuestLogin', {
        device_id: get().deviceId,
        username,
      })) as { player_id: number; username: string } | { code: number; message: string };

      if ('code' in res && res.code !== 0) {
        throw new Error(res.message || 'Login failed');
      }

      const player = res as { player_id: number; username: string };
      localStorage.setItem('cg_username', player.username);
      set({ player, isLoggedIn: true, isAuthChecked: true });
      toast.success(`Welcome, ${player.username}!`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Login failed');
      set({ isAuthChecked: true });
      throw err;
    } finally {
      set({ isConnecting: false });
    }
  },

  checkSession: async () => {
    if (get().isConnecting) return;
    const savedUsername = localStorage.getItem('cg_username');
    if (!savedUsername) {
      set({ isAuthChecked: true });
      return;
    }

    set({ isConnecting: true });
    try {
      const client = getNanoClient();
      if (!client.connected) {
        await client.connect();
      }
      // Setup auto-relogin on reconnect
      if (reconnectHandler) {
        client.off('reconnect', reconnectHandler);
      }
      reconnectHandler = () => {
        const saved = localStorage.getItem('cg_username');
        if (saved) {
          get().checkSession();
        }
      };
      client.on('reconnect', reconnectHandler);

      const res = (await client.request('GuestAuth.GuestLogin', {
        device_id: get().deviceId,
        username: savedUsername,
      })) as { player_id: number; username: string } | { code: number; message: string };

      if (!('code' in res)) {
        set({ player: res as { player_id: number; username: string }, isLoggedIn: true });
      }
    } catch {
      // silent fail, user can login again
    } finally {
      set({ isConnecting: false, isAuthChecked: true });
    }
  },

  logout: () => {
    localStorage.removeItem('cg_username');
    resetNanoClient();
    set({ player: null, isLoggedIn: false, isAuthChecked: true });
  },
}));