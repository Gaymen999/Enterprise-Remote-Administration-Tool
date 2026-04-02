import axios, { AxiosError, InternalAxiosRequestConfig } from 'axios';
import type { Agent } from '../types/agent';

const API_BASE_URL = import.meta.env.VITE_API_URL || '/api/v1';
const WS_BASE_URL = import.meta.env.VITE_WS_URL || 'ws://localhost:8080/api/v1/ws';

export const getWebSocketUrl = () => WS_BASE_URL;

interface QueueItem {
  resolve: (value: unknown) => void;
  reject: (reason?: unknown) => void;
}

let isRefreshing = false;
let refreshPromise: Promise<boolean> | null = null;
const requestQueue: QueueItem[] = [];

const processQueue = (error: Error | null, token: string | null = null) => {
  requestQueue.forEach(({ resolve, reject }) => {
    if (error) {
      reject(error);
    } else {
      resolve(token);
    }
  });
  requestQueue.length = 0;
};

const dispatchAuthEvent = (type: 'logout' | 'expired') => {
  window.dispatchEvent(new CustomEvent('auth:event', { detail: { type } }));
};

const formatAxiosError = (error: AxiosError): {
  message: string;
  status?: number;
  code?: string;
  details?: unknown;
} => {
  if (error.response) {
    const status = error.response.status;
    let message = 'An error occurred';

    switch (status) {
      case 400:
        message = error.response.data 
          ? (error.response.data as { error?: string })?.error || 'Bad request'
          : 'Invalid request';
        break;
      case 401:
        message = 'Authentication required';
        break;
      case 403:
        message = 'Access denied';
        break;
      case 404:
        message = 'Resource not found';
        break;
      case 429:
        message = 'Too many requests';
        break;
      case 500:
        message = 'Server error';
        break;
      default:
        message = `Request failed (${status})`;
    }

    return {
      message,
      status,
      code: error.code,
      details: error.response.data,
    };
  }

  if (error.request) {
    return {
      message: 'Network error - server unreachable',
      code: 'NETWORK_ERROR',
    };
  }

  return {
    message: error.message || 'Unknown error',
    code: 'UNKNOWN',
  };
};

export const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
  withCredentials: true,
});

api.interceptors.request.use(
  (config) => {
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

api.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const originalRequest = error.config as InternalAxiosRequestConfig & { _retry?: boolean };
    
    if (!originalRequest) {
      return Promise.reject(formatAxiosError(error));
    }

    if (error.response?.status === 401 && !originalRequest._retry) {
      if (isRefreshing) {
        return new Promise((resolve, reject) => {
          requestQueue.push({ resolve, reject });
        })
          .then(() => api(originalRequest))
          .catch((err) => Promise.reject(formatAxiosError(err as AxiosError)));
      }

      originalRequest._retry = true;
      isRefreshing = true;

      try {
        if (!refreshPromise) {
          refreshPromise = api.post('/auth/refresh')
            .then(() => {
              processQueue(null, 'refreshed');
              return true;
            })
            .catch((refreshError) => {
              processQueue(refreshError as Error, null);
              dispatchAuthEvent('expired');
              return Promise.reject(refreshError);
            })
            .finally(() => {
              isRefreshing = false;
              refreshPromise = null;
            });
        }

        await refreshPromise;
        return api(originalRequest);
      } catch (refreshError) {
        dispatchAuthEvent('expired');
        return Promise.reject(formatAxiosError(refreshError as AxiosError));
      }
    }

    return Promise.reject(formatAxiosError(error));
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
      dispatchAuthEvent('logout');
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
  createCommand: async (
    agentId: string,
    executable: string,
    args: string[] = [],
    timeoutSeconds: number = 300
  ) => {
    const response = await api.post('/commands', {
      agent_id: agentId,
      executable,
      args,
      timeout_seconds: timeoutSeconds,
    });
    return response.data;
  },
};

export type ApiError = ReturnType<typeof formatAxiosError>;