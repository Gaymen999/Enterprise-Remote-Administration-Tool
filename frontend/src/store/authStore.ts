import { create } from 'zustand';
import { api, authService } from '../services/api';

interface User {
  id: string;
  username: string;
  role: string;
}

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  setAuth: (user: User) => void;
  checkAuth: () => Promise<boolean>;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isAuthenticated: false,

  login: async (username: string, password: string) => {
    const response = await authService.login(username, password);
    const { user } = response;
    set({ user, isAuthenticated: true });
  },

  logout: async () => {
    try {
      await authService.logout();
    } finally {
      set({ user: null, isAuthenticated: false });
    }
  },

  setAuth: (user: User) => {
    set({ user, isAuthenticated: true });
  },

  checkAuth: async () => {
    try {
      await authService.refreshToken();
      return true;
    } catch {
      return false;
    }
  },
}));
