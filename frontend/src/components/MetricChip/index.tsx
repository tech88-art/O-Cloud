import type { ReactNode } from 'react';
import styles from './styles.module.css';

export type MetricTrend = 'up' | 'down' | 'flat';
export type MetricTone = 'default' | 'success' | 'warning' | 'danger';

export interface MetricChipProps {
  /** Label, e.g. "Avg AI Core Util". Pass `t('key')` for i18n. */
  label: ReactNode;
  /** Value, e.g. `65` or "N/A". Accepts strings or numbers. */
  value: ReactNode;
  /** Optional unit, rendered as a small suffix (e.g. `%`, `Gi`, `TOPS`). */
  unit?: ReactNode;
  /** Optional trend arrow. `up` -> ↑, `down` -> ↓, `flat` -> →. */
  trend?: MetricTrend;
  /** Optional tone for emphasis. Defaults to `default`. */
  tone?: MetricTone;
  /** Optional className for the chip root. */
  className?: string;
}

const TREND_GLYPH: Record<MetricTrend, string> = {
  up: '↑',
  down: '↓',
  flat: '→',
};

const TONE_CLASS: Record<MetricTone, string> = {
  default: '',
  success: styles.toneSuccess,
  warning: styles.toneWarning,
  danger: styles.toneDanger,
};

const TREND_CLASS: Record<MetricTrend, string> = {
  up: styles.trendUp,
  down: styles.trendDown,
  flat: styles.trendFlat,
};

/**
 * Compact key-value display chip. Use for inline summary metrics, e.g.
 * "Avg AI Core Util: 65% ↑" or "Pods: 12 →".
 *
 * i18n: `label`, `value`, and `unit` accept any `ReactNode`. The component
 * performs no translation lookups (i18n files are outside T-107
 * Allowed Paths; consumers pass translated strings in).
 */
export function MetricChip({
  label,
  value,
  unit,
  trend,
  tone = 'default',
  className,
}: MetricChipProps) {
  const rootCls = [styles.chip, TONE_CLASS[tone], className]
    .filter(Boolean)
    .join(' ');
  return (
    <span
      className={rootCls}
      data-testid="metric-chip"
      data-tone={tone}
      data-trend={trend ?? 'none'}
    >
      <span className={styles.label}>{label}</span>
      <span className={styles.value}>
        {value}
        {unit && <span className={styles.unit}>{unit}</span>}
      </span>
      {trend && (
        <span
          className={`${styles.trend} ${TREND_CLASS[trend]}`}
          data-testid="metric-chip-trend"
          aria-label={`trend ${trend}`}
        >
          {TREND_GLYPH[trend]}
        </span>
      )}
    </span>
  );
}

export default MetricChip;
