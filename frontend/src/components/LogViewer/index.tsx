import { memo, useEffect, useLayoutEffect, useRef } from 'react';
import type { LogLine } from '@/services/logs';

/**
 * LogViewer — monospace, color-by-level log surface (P1-T-302).
 *
 * Responsibilities:
 *   - Render the supplied `lines` array (ordered chronologically ascending).
 *   - Auto-scroll to the bottom when new lines arrive, BUT only when the
 *     user is already pinned at the bottom — preserves manual scroll
 *     position while the operator inspects history.
 *   - Color-code by level (INFO/WARN/ERROR), with a default gray for
 *     unknown levels so a future contract addition doesn't go invisible.
 *
 * Non-responsibilities:
 *   - Data fetching (the Logs page owns react-query + WS plumbing).
 *   - Buffer capping (the page applies the cap before passing lines in).
 *   - Filtering (the page filters / merges REST + WS before render).
 *
 * Why no virtualization: Phase 1 capped at ~1000 lines; a plain `<div>` per
 * line is well under the React reconciliation threshold. PHASE-2 with
 * unbounded retention will migrate to react-window or similar.
 *
 * Memoized so a parent re-render that doesn't change `lines` or `height`
 * doesn't reflow the (potentially long) line list.
 */

export interface LogViewerProps {
  lines: ReadonlyArray<LogLine>;
  /** Pixel height of the viewport. Defaults to `400px` if omitted. */
  height?: number | string;
  /** Optional override for the empty-state message (i18n string). */
  emptyText?: string;
}

const LEVEL_COLOR: Record<string, string> = {
  INFO: '#1f1f1f',
  WARN: '#fa8c16',
  ERROR: '#ff4d4f',
  DEBUG: '#8c8c8c',
};

/** Threshold (in px) within which "scrolled to bottom" is considered true.
 *  AntD scrollbars + rounding errors mean strict `scrollTop + clientHeight ===
 *  scrollHeight` is too brittle; a 4px slack is large enough to be forgiving
 *  and small enough not to trigger when the user has scrolled up. */
const NEAR_BOTTOM_SLACK_PX = 4;

function LogViewerInner({ lines, height = 400, emptyText }: LogViewerProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  // Whether the viewer was scroll-pinned to the bottom on the LAST render.
  // We sample this BEFORE the DOM commits new lines so the post-commit
  // auto-scroll only fires when the user wasn't manually parked elsewhere.
  const pinnedRef = useRef<boolean>(true);

  // Sample the "pinned at bottom" state synchronously before the DOM commit
  // that grows the list. useLayoutEffect fires after commit but before paint,
  // which is too late to capture pre-commit scroll position; useEffect fires
  // even later. Reading inside the render body is safe because we don't write
  // any DOM here.
  if (containerRef.current) {
    const el = containerRef.current;
    pinnedRef.current =
      el.scrollHeight - el.scrollTop - el.clientHeight <= NEAR_BOTTOM_SLACK_PX;
  }

  useLayoutEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    if (!pinnedRef.current) return;
    // Pinning: snap to the new bottom. Using scrollTop = scrollHeight skips
    // smooth animation — for a live tail, instant feels right.
    el.scrollTop = el.scrollHeight;
  }, [lines]);

  // First mount: scroll to bottom unconditionally so the operator sees the
  // newest line first regardless of how lines arrived (REST tail vs WS).
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, []);

  if (lines.length === 0) {
    return (
      <div
        data-testid="log-viewer-empty"
        style={{
          height,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: '#fafafa',
          border: '1px solid #f0f0f0',
          borderRadius: 4,
          color: '#8c8c8c',
          fontSize: 13,
        }}
      >
        {emptyText ?? 'No logs'}
      </div>
    );
  }

  return (
    <div
      ref={containerRef}
      data-testid="log-viewer"
      style={{
        height,
        overflowY: 'auto',
        background: '#0f1419',
        color: '#d4d4d4',
        padding: '8px 12px',
        borderRadius: 4,
        fontFamily:
          '"JetBrains Mono", "Fira Code", "Cascadia Mono", Menlo, Consolas, monospace',
        fontSize: 12,
        lineHeight: 1.5,
      }}
    >
      {lines.map((line, idx) => (
        <LogLineRow key={`${line.timestamp}-${idx}`} line={line} />
      ))}
    </div>
  );
}

interface LogLineRowProps {
  line: LogLine;
}

/**
 * Single-line renderer. Pulled out so the lines map() above stays compact and
 * the row stays a stable component for React keyed reconciliation.
 *
 * Timestamp formatting trims to seconds (HH:mm:ss) so the line fits a
 * typical 1280px viewport without truncation; the full ISO is in the title
 * attribute on hover for ops that want it.
 */
function LogLineRow({ line }: LogLineRowProps) {
  const level = (line.level ?? '').toUpperCase();
  const color = LEVEL_COLOR[level] ?? '#bfbfbf';
  const time = formatTime(line.timestamp);
  return (
    <div
      data-testid="log-viewer-row"
      data-level={level || 'UNKNOWN'}
      style={{
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        display: 'flex',
        gap: 8,
      }}
      title={line.timestamp}
    >
      <span style={{ color: '#8c8c8c', flexShrink: 0 }}>{time}</span>
      <span
        style={{
          color,
          fontWeight: 600,
          minWidth: 48,
          flexShrink: 0,
        }}
      >
        {level || '·'}
      </span>
      {line.container ? (
        <span style={{ color: '#69b1ff', flexShrink: 0 }}>[{line.container}]</span>
      ) : null}
      <span style={{ color: '#e8e8e8', flexGrow: 1 }}>{line.message}</span>
    </div>
  );
}

function formatTime(iso?: string): string {
  if (!iso) return '';
  // Trim "2026-05-18T09:36:45.218Z" → "09:36:45".
  // Cheap split — no Date allocation in hot path. Fallback to raw on shape miss.
  const t = iso.indexOf('T');
  if (t < 0) return iso;
  const tail = iso.slice(t + 1);
  const dot = tail.indexOf('.');
  if (dot > 0) return tail.slice(0, dot);
  const z = tail.indexOf('Z');
  if (z > 0) return tail.slice(0, z);
  return tail;
}

export const LogViewer = memo(LogViewerInner);
export default LogViewer;
