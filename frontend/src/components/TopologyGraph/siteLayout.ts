/**
 * Site-view layout geometry + pure layout helpers for `<TopologyGraph>`
 * (P12 · runbook 视觉北极星 = demo-runbook.html Step 1).
 *
 * Lives in its own `.ts` (no React, no @xyflow) so:
 *   - the component file (`TopologyGraph.tsx`) only exports components, keeping
 *     Fast Refresh happy (react-refresh/only-export-components);
 *   - these pure functions are unit-testable in isolation, with no canvas mock.
 *
 * Single source of truth for the in-card NPU grid geometry — both
 * `layoutWorkerNpus` (positions the NPU dot nodes) and `WorkerCardNode` (draws
 * the HCCS ring enclosures) consume the SAME constants, so the dashed rings
 * line up with the dots at any zoom.
 */

/**
 * P12 site-view layout constants (runbook-style cards + circles). Each worker
 * renders as a card containing its NPUs as a grid of status circles
 * (grouped by HCCS group / NUMA), workers laid out in a horizontal row under
 * the cluster pill.
 */
export const CARD_W = 188;
export const CARD_H = 196;
export const CARD_GAP = 44;
export const NPU_CIRCLE = 26;
export const NPU_GRID_COLS = 4;
/**
 * In-card NPU grid geometry (relative to the worker card's top-left corner).
 * Shared by `layoutWorkerNpus` (dot positions) and `WorkerCardNode` (HCCS ring
 * enclosure) so the ring always aligns with the dots.
 */
export const NPU_GRID_PAD_X = 26;
// 58 (not 50): leaves the top HCCS ring's dashed border + "HCCS-n" caption a
// clean ~9px gap below the card's "N× Ascend 910B" subtitle when drilled in.
export const NPU_GRID_TOP = 58;
export const NPU_GRID_ROW_H = 42;
/** Padding (px) between a HCCS group's NPU-dot bounding box and its ring enclosure. */
export const HCCS_RING_PAD = 7;

/**
 * In-card geometry for one HCCS group's ring overlay, relative to the worker
 * card's top-left corner. `WorkerCardNode` draws a dashed rounded enclosure at
 * (x, y, w, h) around the group's NPU dots when the worker is drilled into —
 * visualising the "HCCS group 共址" scheduling selling point (runbook D6).
 */
export interface HccsRingGeom {
  groupId: string;
  /** Compact caption, e.g. "HCCS-0" — derived from the group id's trailing ordinal. */
  shortLabel: string;
  x: number;
  y: number;
  w: number;
  h: number;
  npuCount: number;
}

/** "worker-site-a-01-hccs-0" → "HCCS-0"; falls back to "HCCS" with no ordinal. */
export function shortHccsLabel(groupId: string): string {
  const m = groupId.match(/(\d+)\s*$/);
  return m ? `HCCS-${m[1]}` : 'HCCS';
}

/**
 * Lay a worker's NPUs out inside its card grouped by `hccsGroup` — each HCCS
 * group occupies its own row(s) — and return both the per-dot positions
 * (relative to the card's top-left) and the enclosing ring box per group.
 *
 * Why group-by-hccsGroup (not raw contains order): an Atlas 800 packs its NPUs
 * into 4/8-card HCCS groups aligned to NUMA nodes (configs/CLAUDE.md §3.3).
 * Putting each group on its own row makes "these N NPUs share one HCCS ring"
 * legible and lets the card draw a single enclosure per group. For set-a-small
 * (2 groups × 4, indexed 0-7) this is byte-identical to the previous index-order
 * 2×4 grid, so the un-drilled site view is visually unchanged.
 *
 * A ring is emitted only for a *named* group with ≥2 members — a lone NPU has no
 * intra-group HCCS peer (mirrors the backend `appendHCCS` rule); NPUs with no
 * hccsGroup still get grid positions but no ring.
 *
 * Pure: same input → same output, no DOM access.
 */
export function layoutWorkerNpus(
  npus: ReadonlyArray<{ id: string; hccsGroup: string; index: number }>,
): { dotRel: Array<{ id: string; x: number; y: number }>; rings: HccsRingGeom[] } {
  const colStep =
    NPU_GRID_COLS > 1
      ? (CARD_W - 2 * NPU_GRID_PAD_X - NPU_CIRCLE) / (NPU_GRID_COLS - 1)
      : 0;

  // Partition into groups by hccsGroup, preserving first-appearance order.
  const order: string[] = [];
  const groups = new Map<string, Array<{ id: string; index: number }>>();
  for (const n of npus) {
    let arr = groups.get(n.hccsGroup);
    if (!arr) {
      arr = [];
      groups.set(n.hccsGroup, arr);
      order.push(n.hccsGroup);
    }
    arr.push({ id: n.id, index: n.index });
  }

  const dotRel: Array<{ id: string; x: number; y: number }> = [];
  const rings: HccsRingGeom[] = [];
  let row = 0;
  for (const key of order) {
    // Sort members within a group by NPU index so the layout is deterministic.
    const members = groups.get(key)!.slice().sort((a, b) => a.index - b.index);
    const startRow = row;
    members.forEach((m, j) => {
      const col = j % NPU_GRID_COLS;
      const r = startRow + Math.floor(j / NPU_GRID_COLS);
      dotRel.push({
        id: m.id,
        x: NPU_GRID_PAD_X + col * colStep,
        y: NPU_GRID_TOP + r * NPU_GRID_ROW_H,
      });
    });
    row = startRow + Math.ceil(members.length / NPU_GRID_COLS);

    if (key !== '' && members.length >= 2) {
      const cols = Math.min(members.length, NPU_GRID_COLS);
      const minX = NPU_GRID_PAD_X;
      const maxX = NPU_GRID_PAD_X + (cols - 1) * colStep + NPU_CIRCLE;
      const minY = NPU_GRID_TOP + startRow * NPU_GRID_ROW_H;
      const maxY = NPU_GRID_TOP + (row - 1) * NPU_GRID_ROW_H + NPU_CIRCLE;
      rings.push({
        groupId: key,
        shortLabel: shortHccsLabel(key),
        x: minX - HCCS_RING_PAD,
        y: minY - HCCS_RING_PAD,
        w: maxX - minX + 2 * HCCS_RING_PAD,
        h: maxY - minY + 2 * HCCS_RING_PAD,
        npuCount: members.length,
      });
    }
  }
  return { dotRel, rings };
}
