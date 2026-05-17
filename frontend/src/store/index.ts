import { create } from 'zustand';
import type { Locale } from '@/i18n';

/**
 * App-wide UI store. Per frontend/CLAUDE.md §4.3 — Zustand only, no Redux/Recoil.
 *
 * Page-scoped stores (e.g. topologyStore) will live alongside the page that
 * owns them. This file holds cross-page UI state.
 */
interface AppState {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  siderCollapsed: boolean;
  toggleSider: () => void;
}

export const useAppStore = create<AppState>((set) => ({
  locale: 'zh-CN',
  setLocale: (locale) => set({ locale }),
  siderCollapsed: false,
  toggleSider: () => set((s) => ({ siderCollapsed: !s.siderCollapsed })),
}));
