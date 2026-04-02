import axios from 'axios';
import type { Agent } from '../types/agent';

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080/api/v1';
const WS_BASE_URL = import.meta.env.VITE_WS_URL || 'ws://localhost:8080/api/v1/ws';

export const getWebSocketUrl = () => WS_BASE_URL;

export const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
  withCredentials: true,
});

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const originalRequest = error.config;
    
    if (error.response?.status === 401 && !originalRequest._retry) {
      originalRequest._retry = true;
      
      try {
        await api.post('/auth/refresh');
        return api(originalRequest);
      } catch (refreshError) {
        window.location.href = '/login';
        return Promise.reject(refreshError);
      }
    }
    return Promise.reject(error);
  }
);

export const authService = {
  login: async (username: string, password: string) => {
    const response = await api.post('/auth/login', { username, password });
    return response.data;
  },
  logout: async () => {
    try {
      await api.post('/auth/logout');
    } finally {
      window.location.href = '/login';
    }
  },
  refreshToken: async () => {
    await api.post('/auth/refresh');
  },
};

export const agentService = {
  getAgents: async (): Promise<Agent[]> => {
    const response = await api.get('/agents');
    return response.data.agents || [];
  },
  getAgent: async (id: string): Promise<Agent> => {
    const response = await api.get(`/agents/${id}`);
    return response.data;
  },
  createCommand: async (agentId: string, executable: string, args: string[] = [], timeoutSeconds: number = 300) => {
    const response = await api.post('/commands', {
      agent_id: agentId,
      executable,
      args,
      timeout_seconds: timeoutSeconds,
    });
    return response.data;
  },
};
