import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { AppLayout } from '@/components/Layout';
import OverviewPage from '@/pages/Overview';

/**
 * Top-level router (P12-T-205 · one-page workspace).
 *
 * The app is a single workspace: `/overview` IS that workspace and `/`
 * redirects to it. The standalone `/workloads /deploy /metrics /logs` pages
 * (and the `/poc/topology` dev route) were retired here — their logic folded
 * into the workspace's right pane (DetailPanel metrics + logs sections) and
 * the top preset bar. The catch-all redirects any stale deep-link to the
 * workspace so old bookmarks land on `/overview` rather than 404
 * (ADR-0022 §2.4).
 */
export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<AppLayout />}>
          <Route index element={<Navigate to="/overview" replace />} />
          <Route path="overview" element={<OverviewPage />} />
          <Route path="*" element={<Navigate to="/overview" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
