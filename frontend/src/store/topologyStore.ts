import { create } from 'zustand';

/**
 * Overview page selection state.
 *
 * Four pieces of state live here:
 *
 *   - `selectedClusterId` — which cluster's topology is loaded (driven by
 *     the left tree's root pick / future cluster switcher).
 *   - `selectedNodeId`    — which topology node is highlighted (driven by
 *     both the left tree click and the ReactFlow node click; they share
 *     the same store so either side highlights the same id).
 *   - `expandedNPUs`      — set of NPU ids whose slice children are
 *     currently visible in the topology graph. Toggled by dbl-clicking
 *     an NPU node (P1-T-108b). Slices are still in the topology DTO
 *     (depth=slice) but only rendered when their parent NPU id is in
 *     this set, so the graph isn't drowned by N×M slice nodes at idle.
 *   - `lastEventAt`       — ISO timestamp of the most recent WS event
 *     applied to the store. Surfaced in the WS-status indicator so
 *     operators can see the stream is alive.
 *
 * Per frontend/CLAUDE.md §4.3 — Zustand only. Page-scoped store kept here
 * (not on the cross-page `store/index.ts`) so that other pages don't
 * accidentally read overview-only state.
 */
export interface TopologyState {
  selectedClusterId: string | null;
  selectedNodeId: string | null;
  /** NPU ids whose slice subtree is currently expanded (dbl-clicked). */
  expandedNPUs: Set<string>;
  /** ISO timestamp of the last WS event the store accepted, or null. */
  lastEventAt: string | null;
  setSelectedCluster: (id: string | null) => void;
  setSelectedNode: (id: string | null) => void;
  /** Toggle whether the given NPU's slice subtree is expanded. */
  toggleExpandedNPU: (npuId: string) => void;
  /** Replace the expanded-NPU set wholesale (e.g. collapse-all). */
  setExpandedNPUs: (ids: Set<string>) => void;
  /** Update `lastEventAt`. Called by the WS hook on each event. */
  setLastEventAt: (iso: string | null) => void;
}

export const useTopologyStore = create<TopologyState>((set) => ({
  selectedClusterId: null,
  selectedNodeId: null,
  expandedNPUs: new Set<string>(),
  lastEventAt: null,
  setSelectedCluster: (id) => set({ selectedClusterId: id }),
  setSelectedNode: (id) => set({ selectedNodeId: id }),
  toggleExpandedNPU: (npuId) =>
    set((state) => {
      // Always allocate a new Set so React-style equality checks see the
      // change; Zustand's shallow comparator won't notice in-place mutation.
      const next = new Set(state.expandedNPUs);
      if (next.has(npuId)) {
        next.delete(npuId);
      } else {
        next.add(npuId);
      }
      return { expandedNPUs: next };
    }),
  setExpandedNPUs: (ids) => set({ expandedNPUs: new Set(ids) }),
  setLastEventAt: (iso) => set({ lastEventAt: iso }),
}));
