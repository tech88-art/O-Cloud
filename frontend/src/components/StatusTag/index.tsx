import { Tag } from 'antd';
import type { ReactNode } from 'react';

/**
 * Allowed AntD `Tag` color tokens we use as the status palette.
 *
 * AntD `Tag` accepts arbitrary colors, but we restrict to a small named set
 * so that callers and the built-in map stay consistent.
 */
export type StatusTone =
  | 'success'
  | 'processing'
  | 'warning'
  | 'error'
  | 'default';

/**
 * Status -> tone mapping. Keys are lowercased on lookup, so callers can pass
 * `Ready`, `READY`, or `ready` interchangeably.
 *
 * The built-in map covers three common status vocabularies:
 *
 * - Cluster / node health:   healthy / degraded / faulty / offline
 * - K8s node Ready condition: Ready / NotReady / Unknown
 * - Pod / job phase:         running / pending / succeeded / failed / unknown
 *
 * Callers can supply `mapping` to override or extend this for domain-specific
 * status vocabularies (e.g. workload phases not in this list).
 */
// Constant exported alongside the component. The react-refresh hint about
// HMR boundaries is intentionally suppressed here — the map is a small,
// read-only lookup table and splitting it into a separate module would only
// add indirection.
// eslint-disable-next-line react-refresh/only-export-components
export const DEFAULT_STATUS_MAP: Readonly<Record<string, StatusTone>> = {
  // health vocabulary
  healthy: 'success',
  degraded: 'warning',
  faulty: 'error',
  offline: 'default',
  // k8s node Ready vocabulary
  ready: 'success',
  notready: 'error',
  unknown: 'default',
  // pod / job phase vocabulary
  running: 'processing',
  pending: 'warning',
  succeeded: 'success',
  failed: 'error',
  // additional common states
  active: 'success',
  inactive: 'default',
  warning: 'warning',
  error: 'error',
};

export interface StatusTagProps {
  /**
   * Status string. Matched case-insensitively against the mapping.
   * Unrecognised values fall back to the `default` (gray) tone.
   */
  status: string;
  /**
   * Optional per-call override. Merged on top of the built-in map.
   * Use this for domain-specific status vocabularies that aren't covered
   * by the built-in set.
   */
  mapping?: Record<string, StatusTone>;
  /**
   * Optional display label. Defaults to the raw `status` string (so callers
   * keep control of capitalisation / i18n by passing a translated label).
   */
  label?: ReactNode;
}

/**
 * Color-coded AntD `Tag` for status enums.
 *
 * i18n: the visible text is the `status` string by default. To localise,
 * pass `label={t('myKey')}`. The component does not perform i18n lookups
 * itself (see commit message: i18n files are outside T-107 Allowed Paths).
 */
export function StatusTag({ status, mapping, label }: StatusTagProps) {
  const key = status.toLowerCase().replace(/[\s_-]+/g, '');
  const merged: Record<string, StatusTone> = mapping
    ? { ...DEFAULT_STATUS_MAP, ...lowercaseKeys(mapping) }
    : DEFAULT_STATUS_MAP;
  const tone: StatusTone = merged[key] ?? 'default';
  return (
    <Tag color={tone} data-testid="status-tag" data-tone={tone}>
      {label ?? status}
    </Tag>
  );
}

function lowercaseKeys(
  m: Record<string, StatusTone>,
): Record<string, StatusTone> {
  const out: Record<string, StatusTone> = {};
  for (const [k, v] of Object.entries(m)) {
    out[k.toLowerCase().replace(/[\s_-]+/g, '')] = v;
  }
  return out;
}

export default StatusTag;
