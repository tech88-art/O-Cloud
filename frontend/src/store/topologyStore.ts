import { create } from 'zustand';

/**
 * Overview page selection state.
 *
 * Two pieces of state live here:
 *
 *   - `selectedClusterId` — which cluster's topology is loaded (driven by
 *     the left tree's root pick / future cluster switcher).
 *   - `selectedNodeId`    — which topology node is highlighted (driven by
 *     both the left tree click and the ReactFlow node click; they share
 *     the same store so either side highlights the same id).
 *
 * Per frontend/CLAUDE.md §4.3 — Zustand only. Page-scoped store kept here
 * (not on the cross-page `store/index.ts`) so that other pages don't
 * accidentally read overview-only state.
 *
 * T-108b will extend this store with detail-panel + WS-driven node-state
 * cache. We keep the surface narrow on purpose for T-108a.
 */
export interface TopologyState {
  selectedClusterId: string | null;
  selectedNodeId: string | null;
  setSelectedCluster: (id: string | null) => void;
  setSelectedNode: (id: string | null) => void;
}

export const useTopologyStore = create<TopologyState>((set) => ({
  selectedClusterId: null,
  selectedNodeId: null,
  setSelectedCluster: (id) => set({ selectedClusterId: id }),
  setSelectedNode: (id) => set({ selectedNodeId: id }),
}));
