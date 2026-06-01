/**
 * TopologyGraph module entry.
 *
 * The default export is the production ReactFlow wrapper used by the Overview
 * workspace. The P1-T-009 POC artefacts (`G6POC` / `ReactFlowPOC` / `POCPage`
 * / `poc-fixture` / `useFpsMeter`) and the `/poc/topology` dev route were
 * retired in P12-T-205 — ReactFlow won the selection, so the comparison
 * scaffold is gone.
 */
export { TopologyGraph, default } from './TopologyGraph';
export type { TopologyGraphProps } from './TopologyGraph';
