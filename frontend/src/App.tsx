import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { AppLayout } from '@/components/Layout';
import OverviewPage from '@/pages/Overview';
import WorkloadsPage from '@/pages/Workloads';
import DeployPage from '@/pages/Deploy';
import MetricsPage from '@/pages/Metrics';
import POCPage from '@/components/TopologyGraph/POCPage';

/**
 * Top-level router. 5 pages per frontend/CLAUDE.md §1. Root path redirects
 * to /overview.
 *
 * `/poc/topology` is a dev-time route added by P1-T-009 to evaluate G6
 * vs ReactFlow. It is not linked from the main navigation; remove or
 * fold into Overview once P1-T-108a picks a winner.
 *
 * NOTE (P1-T-009): the LogsPage import was removed because
 * `src/pages/Logs/` was never committed in P1-T-006 (typecheck was
 * broken on `dev` before this PR). The /logs route falls through to the
 * catch-all and lands on /overview until P1-T-302 adds the real page.
 */
export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<AppLayout />}>
          <Route index element={<Navigate to="/overview" replace />} />
          <Route path="overview" element={<OverviewPage />} />
          <Route path="workloads" element={<WorkloadsPage />} />
          <Route path="deploy" element={<DeployPage />} />
          <Route path="metrics" element={<MetricsPage />} />
          <Route path="poc/topology" element={<POCPage />} />
          <Route path="*" element={<Navigate to="/overview" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
