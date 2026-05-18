import { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, InputNumber, Select, Skeleton, Space, Switch, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { EmptyState } from '@/components/EmptyState';
import { ErrorState } from '@/components/ErrorState';
import { LogViewer } from '@/components/LogViewer';
import { useLogsWS } from '@/hooks/useLogsWS';
import { useWorkloads, useWorkloadDetail } from '@/services/workload';
import { useWorkloadLogs, type LogLine } from '@/services/logs';

const { Title, Text } = Typography;

/**
 * Logs page (P1-T-302).
 *
 * Layout:
 *   - Top toolbar: workload picker / container picker / tail size / live toggle
 *   - Main: LogViewer (monospace, level-coloured, auto-scroll-on-append)
 *
 * Behaviour:
 *   - Picking a workload triggers a REST tail fetch via useWorkloadLogs.
 *   - "Live stream" toggle ON connects /ws/logs/:ns/:name; incoming lines
 *     are appended to the local buffer alongside the REST tail.
 *   - Local buffer is capped at MAX_LINES — oldest lines drop. This is the
 *     long-session OOM guard.
 *
 * Why not Zustand: logs are page-scoped (no other page reads them) and
 * the buffer churn is high (1 line/s in live mode). Per frontend/CLAUDE.md
 * §4.3 cross-page state goes through Zustand; this stays in component state.
 */

/** Hard cap on lines retained in the in-page buffer. */
const MAX_LINES = 1000;

/** Default tail size when the user opens the page. Matches the OpenAPI default. */
const DEFAULT_TAIL = 200;

/** Tail size choices in the dropdown. 2000 matches the backend's logsRESTTailMax. */
const TAIL_CHOICES = [50, 100, 200, 500, 1000, 2000] as const;

export default function LogsPage() {
  const { t } = useTranslation();
  // Workload selection state. `workloadKey` is "namespace/name" so the AntD
  // <Select> value is a stable string; we split it before passing to the hooks.
  const [workloadKey, setWorkloadKey] = useState<string | null>(null);
  const [containerFilter, setContainerFilter] = useState<string>('');
  const [tail, setTail] = useState<number>(DEFAULT_TAIL);
  const [live, setLive] = useState<boolean>(false);

  const { namespace, name } = useMemo(() => splitWorkloadKey(workloadKey), [workloadKey]);

  // Workload list for the picker.
  const workloadsQuery = useWorkloads();
  // Workload detail to discover container names — only fires once a workload
  // is picked. The container <Select> reads from this.
  const detailQuery = useWorkloadDetail(namespace, name);

  // REST tail. Refetches on any of (namespace, name, container, tail) change.
  const logsQuery = useWorkloadLogs(namespace, name, {
    container: containerFilter,
    tail,
  });

  // The line buffer. The REST tail seeds it; WS appends. We never mutate the
  // array in place — every push allocates a new one so React re-renders.
  const [buffer, setBuffer] = useState<LogLine[]>([]);
  // Track which REST response we've already absorbed — avoids re-seeding on
  // a refetch that returns the same response shape.
  const seededRef = useRef<string | null>(null);

  // Seed the buffer from REST whenever logsQuery.data lands. Reset to the
  // REST tail (not append) so the operator sees a clean recent history when
  // they switch workloads / tail size / container.
  useEffect(() => {
    if (!logsQuery.data) return;
    const fingerprint = `${namespace ?? ''}/${name ?? ''}/${containerFilter}/${tail}`;
    if (seededRef.current === fingerprint) return;
    seededRef.current = fingerprint;
    setBuffer(logsQuery.data.lines ?? []);
  }, [logsQuery.data, namespace, name, containerFilter, tail]);

  // Clear buffer on workload change so the operator never sees stale lines
  // from the previous workload while a new tail is in flight.
  useEffect(() => {
    setBuffer([]);
    seededRef.current = null;
  }, [namespace, name]);

  // WS appender. Push each incoming line, cap the buffer at MAX_LINES.
  const wsResult = useLogsWS(
    namespace,
    name,
    live,
    (line) => {
      setBuffer((prev) => {
        const next = prev.length >= MAX_LINES ? prev.slice(-MAX_LINES + 1) : prev.slice();
        next.push(line);
        return next;
      });
    },
    containerFilter ? { container: containerFilter } : undefined,
  );

  return (
    <div data-testid="logs-page">
      <Title level={2}>{t('page.logs.title')}</Title>
      <Toolbar
        workloads={workloadsQuery.data ?? []}
        workloadsLoading={workloadsQuery.isLoading}
        selectedKey={workloadKey}
        onSelectWorkload={(k) => setWorkloadKey(k)}
        containerOptions={collectContainers(detailQuery.data)}
        containerFilter={containerFilter}
        onContainerFilterChange={setContainerFilter}
        tail={tail}
        onTailChange={setTail}
        live={live}
        onLiveChange={setLive}
        wsStatus={wsResult.status}
        labels={{
          workload: t('page.logs.workload'),
          workloadPlaceholder: t('page.logs.workloadPlaceholder'),
          container: t('page.logs.container'),
          containerAll: t('page.logs.containerAll'),
          tail: t('page.logs.tail'),
          live: t('page.logs.live'),
          wsIdle: t('ws.idle'),
          wsConnecting: t('ws.connecting'),
          wsOpen: t('ws.open'),
          wsClosed: t('ws.closed'),
        }}
      />
      <Body
        hasWorkload={Boolean(workloadKey)}
        loading={logsQuery.isLoading}
        error={(logsQuery.error as Error | null) ?? null}
        lines={buffer}
        wsError={wsResult.lastError}
        labels={{
          selectWorkload: t('page.logs.selectWorkload'),
          errorTitle: t('page.logs.errorTitle'),
          retry: t('common.retry'),
          empty: t('page.logs.empty'),
          wsWarn: t('page.logs.wsWarning'),
        }}
        onRetry={() => {
          void logsQuery.refetch();
        }}
      />
    </div>
  );
}

interface ToolbarLabels {
  workload: string;
  workloadPlaceholder: string;
  container: string;
  containerAll: string;
  tail: string;
  live: string;
  wsIdle: string;
  wsConnecting: string;
  wsOpen: string;
  wsClosed: string;
}

interface ToolbarProps {
  workloads: { namespace: string; name: string }[];
  workloadsLoading: boolean;
  selectedKey: string | null;
  onSelectWorkload: (key: string | null) => void;
  containerOptions: string[];
  containerFilter: string;
  onContainerFilterChange: (v: string) => void;
  tail: number;
  onTailChange: (v: number) => void;
  live: boolean;
  onLiveChange: (v: boolean) => void;
  wsStatus: 'idle' | 'connecting' | 'open' | 'closed';
  labels: ToolbarLabels;
}

function Toolbar({
  workloads,
  workloadsLoading,
  selectedKey,
  onSelectWorkload,
  containerOptions,
  containerFilter,
  onContainerFilterChange,
  tail,
  onTailChange,
  live,
  onLiveChange,
  wsStatus,
  labels,
}: ToolbarProps) {
  const wsLabel: Record<typeof wsStatus, string> = {
    idle: labels.wsIdle,
    connecting: labels.wsConnecting,
    open: labels.wsOpen,
    closed: labels.wsClosed,
  };
  const wsTone: Record<typeof wsStatus, 'secondary' | 'success' | 'warning' | 'danger'> = {
    idle: 'secondary',
    connecting: 'warning',
    open: 'success',
    closed: 'danger',
  };
  return (
    <Space wrap style={{ margin: '12px 0', width: '100%' }} size="middle">
      <span>
        <Text type="secondary" style={{ marginRight: 8 }}>
          {labels.workload}:
        </Text>
        <Select
          data-testid="logs-workload-select"
          showSearch
          allowClear
          loading={workloadsLoading}
          value={selectedKey ?? undefined}
          placeholder={labels.workloadPlaceholder}
          style={{ minWidth: 280 }}
          options={workloads.map((w) => ({
            value: `${w.namespace}/${w.name}`,
            label: `${w.namespace}/${w.name}`,
          }))}
          onChange={(v) => onSelectWorkload((v as string | undefined) ?? null)}
          filterOption={(input, opt) =>
            String(opt?.label ?? '').toLowerCase().includes(input.toLowerCase())
          }
        />
      </span>
      <span>
        <Text type="secondary" style={{ marginRight: 8 }}>
          {labels.container}:
        </Text>
        <Select
          data-testid="logs-container-select"
          allowClear
          disabled={containerOptions.length === 0}
          value={containerFilter || undefined}
          placeholder={labels.containerAll}
          style={{ minWidth: 140 }}
          options={containerOptions.map((c) => ({ value: c, label: c }))}
          onChange={(v) => onContainerFilterChange((v as string | undefined) ?? '')}
        />
      </span>
      <span>
        <Text type="secondary" style={{ marginRight: 8 }}>
          {labels.tail}:
        </Text>
        <InputNumber
          data-testid="logs-tail-input"
          min={1}
          max={2000}
          value={tail}
          onChange={(v) => {
            if (typeof v === 'number' && v > 0) onTailChange(v);
          }}
          style={{ width: 100 }}
        />
        <Select
          data-testid="logs-tail-preset"
          value={tail}
          style={{ marginLeft: 4, width: 80 }}
          options={TAIL_CHOICES.map((n) => ({ value: n, label: String(n) }))}
          onChange={(v) => onTailChange(v as number)}
        />
      </span>
      <span>
        <Text type="secondary" style={{ marginRight: 8 }}>
          {labels.live}:
        </Text>
        <Switch
          data-testid="logs-live-switch"
          checked={live}
          onChange={onLiveChange}
        />
        <Text type={wsTone[wsStatus]} style={{ marginLeft: 8 }} data-testid="logs-ws-status">
          ● {wsLabel[wsStatus]}
        </Text>
      </span>
    </Space>
  );
}

interface BodyLabels {
  selectWorkload: string;
  errorTitle: string;
  retry: string;
  empty: string;
  wsWarn: string;
}

interface BodyProps {
  hasWorkload: boolean;
  loading: boolean;
  error: Error | null;
  lines: LogLine[];
  wsError?: Error;
  labels: BodyLabels;
  onRetry: () => void;
}

function Body({
  hasWorkload,
  loading,
  error,
  lines,
  wsError,
  labels,
  onRetry,
}: BodyProps) {
  if (!hasWorkload) {
    return <EmptyState description={labels.selectWorkload} />;
  }
  if (loading) {
    return <Skeleton active paragraph={{ rows: 8 }} title={false} />;
  }
  if (error) {
    return (
      <ErrorState
        error={error}
        title={labels.errorTitle}
        retryLabel={labels.retry}
        onRetry={onRetry}
      />
    );
  }
  return (
    <>
      {wsError ? (
        <Alert
          data-testid="logs-ws-error"
          type="warning"
          showIcon
          message={labels.wsWarn}
          style={{ marginBottom: 8 }}
        />
      ) : null}
      <LogViewer lines={lines} height={520} emptyText={labels.empty} />
    </>
  );
}

/** Split a "namespace/name" key into its components. Tolerant of nulls. */
function splitWorkloadKey(key: string | null): {
  namespace: string | null;
  name: string | null;
} {
  if (!key) return { namespace: null, name: null };
  const slash = key.indexOf('/');
  if (slash < 0) return { namespace: null, name: null };
  return {
    namespace: key.slice(0, slash),
    name: key.slice(slash + 1),
  };
}

/** Collect distinct container names from the workload detail's first pod —
 *  the backend log generator uses the same heuristic so this aligns with
 *  what the lines actually carry. */
function collectContainers(
  detail: { pods?: { containers?: { name?: string }[] }[] } | undefined,
): string[] {
  if (!detail?.pods || detail.pods.length === 0) return [];
  const first = detail.pods[0];
  if (!first?.containers) return [];
  const seen = new Set<string>();
  for (const c of first.containers) {
    if (c.name && !seen.has(c.name)) seen.add(c.name);
  }
  return Array.from(seen);
}
