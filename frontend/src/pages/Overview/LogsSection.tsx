import { useEffect, useMemo, useRef, useState } from 'react';
import { Select, Skeleton, Switch, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { EmptyState } from '@/components/EmptyState';
import { ErrorState } from '@/components/ErrorState';
import { LogViewer } from '@/components/LogViewer';
import { useLogsWS } from '@/hooks/useLogsWS';
import { useWorkloadDetail } from '@/services/workload';
import { useWorkloadLogs, type LogLine } from '@/services/logs';
import { useTopologyStore } from '@/store/topologyStore';
import sectionStyles from './sections.module.css';

const { Text } = Typography;

/** Hard cap on retained lines (long-session OOM guard · matches Logs page). */
const MAX_LINES = 1000;
const TAIL = 200;

export interface LogsSectionProps {
  namespace: string;
  /** Workload name whose log stream to tail (a pod resolves to its parent). */
  workloadName: string;
}

/**
 * Right-panel logs section (P12-T-203). Tails the selected workload's logs
 * (REST seed + optional live WS) with a container filter — the Logs page's
 * mechanics folded into a collapsible right-panel card. Only the DetailPanel's
 * workload/pod branch mounts it (resources have no business logs · ADR-0022
 * §2.3). The WS stays closed unless the section is open AND live is on.
 */
export function LogsSection({ namespace, workloadName }: LogsSectionProps) {
  const { t } = useTranslation();
  const open = useTopologyStore((s) => s.logsSectionOpen);
  const setOpen = useTopologyStore((s) => s.setLogsSectionOpen);
  const container = useTopologyStore((s) => s.selectedContainer);
  const setContainer = useTopologyStore((s) => s.setSelectedContainer);
  const [live, setLive] = useState(false);

  const detailQuery = useWorkloadDetail(namespace, workloadName);
  const containerOptions = useMemo(
    () => collectContainers(detailQuery.data),
    [detailQuery.data],
  );

  const logsQuery = useWorkloadLogs(namespace, workloadName, {
    container: container ?? '',
    tail: TAIL,
  });

  const [buffer, setBuffer] = useState<LogLine[]>([]);
  const seededRef = useRef<string | null>(null);

  // Seed buffer from the REST tail when it lands (once per ns/name/container).
  useEffect(() => {
    if (!logsQuery.data) return;
    const fp = `${namespace}/${workloadName}/${container ?? ''}`;
    if (seededRef.current === fp) return;
    seededRef.current = fp;
    setBuffer(logsQuery.data.lines ?? []);
  }, [logsQuery.data, namespace, workloadName, container]);

  // On workload change: clear buffer + reset the container filter so the
  // previous workload's container choice doesn't leak.
  useEffect(() => {
    setBuffer([]);
    seededRef.current = null;
    setContainer(null);
  }, [namespace, workloadName, setContainer]);

  // Live WS appender — only when the section is open and live is on.
  useLogsWS(
    namespace,
    workloadName,
    live && open,
    (line) => {
      setBuffer((prev) => {
        const next = prev.length >= MAX_LINES ? prev.slice(-MAX_LINES + 1) : prev.slice();
        next.push(line);
        return next;
      });
    },
    container ? { container } : undefined,
  );

  return (
    <section data-testid="logs-section" className={sectionStyles.section}>
      <header className={sectionStyles.sectionHeader}>
        <Text strong>{t('overview.section.logs')}</Text>
        <Switch
          size="small"
          checked={open}
          onChange={setOpen}
          data-testid="logs-section-toggle"
          aria-label={t('overview.section.logs')}
        />
      </header>
      {open && (
        <div className={sectionStyles.sectionBody} data-testid="logs-section-body">
          <div className={sectionStyles.logsToolbar}>
            <Select
              size="small"
              allowClear
              disabled={containerOptions.length === 0}
              value={container ?? undefined}
              placeholder={t('page.logs.containerAll')}
              style={{ minWidth: 150 }}
              data-testid="logs-section-container"
              options={containerOptions.map((c) => ({ value: c, label: c }))}
              onChange={(v) => setContainer((v as string | undefined) ?? null)}
            />
            <span>
              <Text type="secondary" style={{ marginRight: 6 }}>
                {t('page.logs.live')}
              </Text>
              <Switch
                size="small"
                checked={live}
                onChange={setLive}
                data-testid="logs-section-live"
              />
            </span>
          </div>
          {logsQuery.isLoading ? (
            <Skeleton active paragraph={{ rows: 5 }} title={false} />
          ) : logsQuery.error ? (
            <ErrorState
              error={logsQuery.error as Error}
              title={t('page.logs.errorTitle')}
              retryLabel={t('common.retry')}
              onRetry={() => void logsQuery.refetch()}
            />
          ) : buffer.length === 0 ? (
            <EmptyState description={t('page.logs.empty')} />
          ) : (
            <LogViewer lines={buffer} height={260} emptyText={t('page.logs.empty')} />
          )}
        </div>
      )}
    </section>
  );
}

/** Distinct container names from the workload detail's first pod (matches the
 *  backend log generator heuristic · same as the Logs page). */
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

export default LogsSection;
