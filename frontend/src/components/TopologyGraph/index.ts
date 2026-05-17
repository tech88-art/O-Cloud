/**
 * P1-T-009 — TopologyGraph module entry.
 *
 * For the POC phase this re-exports the two candidate POCs and the
 * fixture. The production wrapper (P1-T-108a) will replace these exports
 * with a single G6/ReactFlow-backed component once we pick a winner.
 */
export { default as G6POC } from './G6POC';
export { default as ReactFlowPOC } from './ReactFlowPOC';
export { default as POCPage } from './POCPage';
export { POC_FIXTURE, makeStressFixture, STATUS_COLOR, TYPE_SIZE } from './poc-fixture';
export type { PocFixture, PocNode, PocEdge, NodeType, NodeStatus } from './poc-fixture';
