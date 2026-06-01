import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { AppLayout } from '@/components/Layout';
import OverviewPage from '@/pages/Overview';
import WorkloadsPage from '@/pages/Workloads';
import DeployPage from '@/pages/Deploy';
import MetricsPage from '@/pages/Metrics';
import LogsPage from '@/pages/Logs';
import POCPage from '@/components/TopologyGraph/POCPage';

/**
 * Top-level router.
 *
 * P12-T-201 / ADR-0022: the app is now a single-page workspace. `/overview`
 * is that workspace (the old left-nav menu is gone), and `/` redirects to it.
 * The standalone `/workloads /deploy /metrics /logs` pages are no longer
 * reachable from the UI — their logic folds into the workspace's right
 * pane / preset bar across T203/T204. They stay routed (deep-link safe)
 * until T205 retires them; the catch-all then redirects any stale link to
 * the workspace.
 *
 * `/poc/topology` is a dev-time route added by P1-T-009 to evaluate G6
 * vs ReactFlow. ReactFlow won; it's cleaned up in T205.
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
          <Route path="logs" element={<LogsPage />} />
          <Route path="poc/topology" element={<POCPage />} />
          <Route path="*" element={<Navigate to="/overview" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
