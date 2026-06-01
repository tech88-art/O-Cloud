import { useState } from 'react';
import {
  BaseEdge,
  EdgeLabelRenderer,
  getBezierPath,
  type EdgeProps,
} from '@xyflow/react';
import { useTranslation } from 'react-i18next';

/**
 * Custom ReactFlow edge for bandwidth-bearing links (P12-T-202 / ADR-0021):
 * `network` (node↔node), `hccs` (npu↔npu intra-node) and `fabric-link`
 * (node↔switch). Renders the styled path (colour/dash comes from the edge
 * `style` set by `edgeRenderingFor`) plus a hover tooltip showing the
 * link's `bandwidthGBps` / `medium` / `utilization` / `hccsGroup`
 * attributes.
 *
 * Three-state spirit (frontend/CLAUDE.md §4.8): if the backend omitted the
 * bandwidth attributes the tooltip simply doesn't show — the edge still
 * renders, it just has nothing to report on hover. No crash on missing data.
 *
 * The visible path is thin; a wider transparent path sits on top as a
 * forgiving hover hit-area so operators don't have to land exactly on a
 * 1.5px line.
 */
export interface BandwidthEdgeData {
  topoEdgeType: string;
  attributes?: Record<string, unknown>;
  [key: string]: unknown;
}

function asNumber(v: unknown): number | null {
  return typeof v === 'number' && Number.isFinite(v) ? v : null;
}

function asString(v: unknown): string | null {
  return typeof v === 'string' && v.length > 0 ? v : null;
}

export function BandwidthEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style,
  markerEnd,
  data,
}: EdgeProps) {
  const { t } = useTranslation();
  const [hovered, setHovered] = useState(false);
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });

  const d = data as BandwidthEdgeData | undefined;
  const attrs = d?.attributes ?? {};
  const bandwidth = asNumber(attrs.bandwidthGBps);
  const medium = asString(attrs.medium);
  const utilization = asNumber(attrs.utilization);
  const hccsGroup = asString(attrs.hccsGroup);

  const typeLabel = (() => {
    switch (d?.topoEdgeType) {
      case 'network':
        return t('topology.edge.typeLabel.network');
      case 'hccs':
        return t('topology.edge.typeLabel.hccs');
      case 'fabric-link':
        return t('topology.edge.typeLabel.fabricLink');
      default:
        return t('topology.edge.typeLabel.generic');
    }
  })();

  const hasTooltip =
    bandwidth !== null || medium !== null || utilization !== null || hccsGroup !== null;

  return (
    <>
      <BaseEdge id={id} path={edgePath} style={style} markerEnd={markerEnd} />
      <path
        d={edgePath}
        fill="none"
        stroke="transparent"
        strokeWidth={16}
        style={{ cursor: hasTooltip ? 'help' : 'default' }}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        data-testid={`bandwidth-edge-hit-${id}`}
      />
      {hovered && hasTooltip && (
        <EdgeLabelRenderer>
          <div
            data-testid="bandwidth-edge-tooltip"
            className="nodrag nopan"
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
              pointerEvents: 'none',
              background: 'rgba(0, 0, 0, 0.82)',
              color: '#fff',
              padding: '6px 8px',
              borderRadius: 6,
              fontSize: 11,
              lineHeight: 1.5,
              whiteSpace: 'nowrap',
              boxShadow: '0 2px 8px rgba(0, 0, 0, 0.25)',
              zIndex: 10,
            }}
          >
            <div style={{ fontWeight: 600 }}>{typeLabel}</div>
            {bandwidth !== null && (
              <div>
                {t('topology.edge.bandwidth')}: {bandwidth} GB/s
              </div>
            )}
            {medium !== null && (
              <div>
                {t('topology.edge.medium')}: {medium}
              </div>
            )}
            {utilization !== null && (
              <div>
                {t('topology.edge.utilization')}: {utilization}%
              </div>
            )}
            {hccsGroup !== null && (
              <div>
                {t('topology.edge.hccsGroup')}: {hccsGroup}
              </div>
            )}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
}

export default BandwidthEdge;
