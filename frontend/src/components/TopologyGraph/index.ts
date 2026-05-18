/**
 * TopologyGraph module entry.
 *
 * T-108a promoted the ReactFlow POC to the production `TopologyGraph` —
 * the default export is the production wrapper used by the Overview page.
 *
 * The P1-T-009 POC files (`G6POC`, `ReactFlowPOC`, `POCPage`) and the
 * shared fixture remain as named exports so the `/poc/topology` dev route
 * stays reachable for cross-library reference until we delete it
 * (tracked separately; not in T-108a scope).
 */
export { TopologyGraph, default } from './TopologyGraph';
export type { TopologyGraphProps } from './TopologyGraph';

// P1-T-009 POC artefacts (kept for reference; not used by production pages)
export { default as G6POC } from './G6POC';
export { default as ReactFlowPOC } from './ReactFlowPOC';
export { default as POCPage } from './POCPage';
export {
  POC_FIXTURE,
  makeStressFixture,
  STATUS_COLOR,
  TYPE_SIZE,
} from './poc-fixture';
export type {
  PocFixture,
  PocNode,
  PocEdge,
  NodeType,
  NodeStatus,
} from './poc-fixture';
