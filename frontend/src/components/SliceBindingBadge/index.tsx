import { Tag, Tooltip } from 'antd';
import type { SliceBinding } from '@/services/workload';

export interface SliceBindingBadgeProps {
  /** The slice binding to render. */
  binding: SliceBinding;
  /** Optional className passthrough. */
  className?: string;
}

const ROLE_TONE: Record<string, string> = {
  prefill: 'magenta',
  decode: 'purple',
  primary: 'blue',
  sidecar: 'cyan',
  init: 'geekblue',
  peer: 'green',
};

/**
 * Compact badge rendering one NPU slice binding (P6-T-103).
 *
 * Displays `<node>/<device> (<aiCores>c)` with the role coloring the
 * Tag. Mirrors the `npu.huawei.com/slice-bindings` annotation format
 * Phase 5 PD Router writes onto PD-pair Pods.
 *
 * Tooltip exposes the full structured tuple (podName, pool, role) for
 * users who need to correlate against scheduler decisions.
 */
export function SliceBindingBadge({
  binding,
  className,
}: SliceBindingBadgeProps) {
  const role = binding.role ?? '';
  const tone = ROLE_TONE[role] ?? 'default';
  const nodeName = binding.nodeName;
  const device = binding.device;
  const aiCores = binding.aiCores ?? 0;
  const label =
    aiCores > 0
      ? `${nodeName}/${device} (${aiCores}c)`
      : `${nodeName}/${device}`;
  const tooltipParts: string[] = [];
  if (binding.podName) tooltipParts.push(`pod: ${binding.podName}`);
  if (binding.pool) tooltipParts.push(`pool: ${binding.pool}`);
  if (role) tooltipParts.push(`role: ${role}`);
  const tooltip = tooltipParts.join(' · ');

  const tagEl = (
    <Tag
      color={tone}
      data-testid="slice-binding-badge"
      data-role={role}
      data-node={nodeName}
      data-device={device}
      className={className}
    >
      {label}
    </Tag>
  );

  if (!tooltip) {
    return tagEl;
  }
  return <Tooltip title={tooltip}>{tagEl}</Tooltip>;
}

export default SliceBindingBadge;
