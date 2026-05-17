import { useState } from 'react';
import G6POC from './G6POC';
import ReactFlowPOC from './ReactFlowPOC';

/**
 * P1-T-009 — Side-by-side POC page mounted at `/poc/topology`.
 *
 * Single route with a tab toggle so the comparison is one click apart
 * for whoever is evaluating (not two browser tabs and visual whiplash).
 * Each tab fully unmounts the other library — important for FPS
 * comparison, since a hidden canvas/ReactFlow renderer would still tick.
 *
 * English-only on purpose: this is an internal dev-time POC page, not a
 * user-facing screen. The two production-page i18n keys we need ("POC"
 * tab label etc.) would otherwise leak into zh-CN.json / en-US.json for
 * a temporary tool. Per task notes: "use English in POC and note in
 * commit".
 */
type Lib = 'g6' | 'reactflow';

export default function POCPage() {
  const [active, setActive] = useState<Lib>('g6');
  const [lastClicked, setLastClicked] = useState<string | null>(null);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: 'calc(100vh - 120px)', gap: 12 }}>
      <header style={{ display: 'flex', gap: 12, alignItems: 'baseline' }}>
        <h2 style={{ margin: 0 }}>Topology Library POC — P1-T-009</h2>
        <span style={{ color: '#8c8c8c' }}>last click: {lastClicked ?? '—'}</span>
      </header>
      <div style={{ display: 'flex', gap: 8 }}>
        <button
          type="button"
          onClick={() => setActive('g6')}
          data-testid="tab-g6"
          style={{
            padding: '4px 12px',
            background: active === 'g6' ? '#1677ff' : '#fff',
            color: active === 'g6' ? '#fff' : '#000',
            border: '1px solid #d9d9d9',
            borderRadius: 4,
            cursor: 'pointer',
          }}
        >
          AntV G6 v5
        </button>
        <button
          type="button"
          onClick={() => setActive('reactflow')}
          data-testid="tab-reactflow"
          style={{
            padding: '4px 12px',
            background: active === 'reactflow' ? '#1677ff' : '#fff',
            color: active === 'reactflow' ? '#fff' : '#000',
            border: '1px solid #d9d9d9',
            borderRadius: 4,
            cursor: 'pointer',
          }}
        >
          ReactFlow v12
        </button>
      </div>
      <div style={{ flex: 1, minHeight: 0 }}>
        {active === 'g6' ? (
          <G6POC onNodeClick={setLastClicked} />
        ) : (
          <ReactFlowPOC onNodeClick={setLastClicked} />
        )}
      </div>
    </div>
  );
}
