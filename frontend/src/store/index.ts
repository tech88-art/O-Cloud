import { create } from 'zustand';
import { persist, createJSONStorage } from 'zustand/middleware';
import type { Locale } from '@/i18n';

/**
 * App-wide UI store. Per frontend/CLAUDE.md §4.3 — Zustand only, no Redux/Recoil.
 *
 * Page-scoped stores (e.g. topologyStore) live alongside the page that owns
 * them. This file holds cross-page UI state.
 *
 * P12-T-201 / ADR-0022: the 5-route + AntSider nav model is gone. The shell
 * is now a single-page workspace whose left (resource tree) and right
 * (detail) panes are independently collapsible + resizable via an AntD
 * `<Splitter>`. The pane layout lives here (not in topologyStore) because the
 * Header toggle buttons (Layout component) write it while the Overview
 * workspace reads it — a cross-page concern.
 *
 * Pane layout is persisted to localStorage so a refresh keeps the operator's
 * chosen widths / collapsed panes (ADR-0022 §4(a)). `merge` validates the
 * persisted shape so a corrupt entry falls back to the default layout
 * (one-page-workspace DESIGN §4) rather than feeding garbage into Splitter.
 */
interface AppState {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  /** Left resource-tree pane hidden (Header toggle / persisted). */
  leftPaneHidden: boolean;
  /** Right detail pane hidden (Header toggle / persisted). */
  rightPaneHidden: boolean;
  toggleLeftPane: () => void;
  toggleRightPane: () => void;
  /** Persisted pane widths in px — used as Splitter `defaultSize` on (re)mount. */
  leftPaneSize: number;
  rightPaneSize: number;
  setPaneSizes: (left: number, right: number) => void;
}

const DEFAULT_LEFT_SIZE = 280;
const DEFAULT_RIGHT_SIZE = 360;
// P12 polish: cap the persisted pane width so a previously dragged-wide pane
// never restores full-screen-wide on reload. AntD Splitter `max` only bounds
// LIVE dragging — it does NOT clamp the `defaultSize` we feed from storage — so
// a stale large value would otherwise squeeze the center stage to nothing on
// load. 600px is a generous sidebar width that still leaves the center usable.
const MAX_PANE_SIZE = 600;
const clampPaneSize = (v: number) => Math.min(Math.max(v, 160), MAX_PANE_SIZE);

export const useAppStore = create<AppState>()(
  persist(
    (set) => ({
      locale: 'zh-CN',
      setLocale: (locale) => set({ locale }),
      leftPaneHidden: false,
      rightPaneHidden: false,
      toggleLeftPane: () => set((s) => ({ leftPaneHidden: !s.leftPaneHidden })),
      toggleRightPane: () => set((s) => ({ rightPaneHidden: !s.rightPaneHidden })),
      leftPaneSize: DEFAULT_LEFT_SIZE,
      rightPaneSize: DEFAULT_RIGHT_SIZE,
      setPaneSizes: (left, right) =>
        set({
          leftPaneSize: clampPaneSize(Math.round(left)),
          rightPaneSize: clampPaneSize(Math.round(right)),
        }),
    }),
    {
      name: 'ocloud-workspace-layout',
      storage: createJSONStorage(() => localStorage),
      // Persist only the pane layout. Locale has its own persistence
      // (i18n persistLocale / loadPersistedLocale), so leaving it out here
      // avoids two sources of truth fighting over the language on reload.
      partialize: (s) => ({
        leftPaneHidden: s.leftPaneHidden,
        rightPaneHidden: s.rightPaneHidden,
        leftPaneSize: s.leftPaneSize,
        rightPaneSize: s.rightPaneSize,
      }),
      // Defensive merge: a hand-edited / corrupted localStorage entry must
      // not crash Splitter. Coerce each field; fall back to defaults.
      merge: (persisted, current) => {
        const p = (persisted ?? {}) as Partial<AppState>;
        const num = (v: unknown, d: number) =>
          typeof v === 'number' && Number.isFinite(v) && v > 0 ? v : d;
        return {
          ...current,
          leftPaneHidden: p.leftPaneHidden === true,
          rightPaneHidden: p.rightPaneHidden === true,
          leftPaneSize: clampPaneSize(num(p.leftPaneSize, DEFAULT_LEFT_SIZE)),
          rightPaneSize: clampPaneSize(num(p.rightPaneSize, DEFAULT_RIGHT_SIZE)),
        };
      },
    },
  ),
);
