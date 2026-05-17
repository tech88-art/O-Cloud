import axios, { type AxiosInstance } from 'axios';
import { QueryClient } from '@tanstack/react-query';
import { getRuntimeConfig } from '@/config/runtime';

/**
 * Shared axios instance. baseURL is resolved at first use from the runtime
 * config (which must have been loaded by main.tsx before any request fires).
 */
export const api: AxiosInstance = axios.create({
  timeout: 30_000,
  headers: { 'Content-Type': 'application/json' },
});

api.interceptors.request.use((config) => {
  if (!config.baseURL) {
    config.baseURL = getRuntimeConfig().apiBaseURL;
  }
  return config;
});

api.interceptors.response.use(
  (res) => res,
  (err: unknown) => {
    return Promise.reject(err);
  },
);

/**
 * Shared react-query client. Sensible defaults — services may override per
 * query (see frontend/CLAUDE.md §4.2).
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 5 * 60_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
    mutations: {
      retry: 0,
    },
  },
});
