import { useEffect, useRef, useState } from 'react';

/**
 * Hook that samples `requestAnimationFrame` to compute rolling FPS.
 *
 * Why roll-your-own instead of pulling in a perf library:
 *   - One file, no dep, no version-pinning headache for a POC.
 *   - The number we care about is "FPS while user interacts" — easy enough
 *     to sample directly. A perf library would give percentiles, but the
 *     comparison table just needs a single representative number per POC.
 *
 * Usage: just call the hook; it starts sampling on mount and stops on
 * unmount. The returned `fps` is the rolling 1-second average rounded to
 * an integer (1 = stable enough for human eyes, distinguishes 60/30/10 fps
 * clearly without pretending sub-Hz precision exists).
 */
export function useFpsMeter(enabled: boolean = true): { fps: number; samples: number } {
  const [fps, setFps] = useState(0);
  const samplesRef = useRef(0);
  const [samples, setSamples] = useState(0);

  useEffect(() => {
    if (!enabled) {
      return;
    }
    let raf = 0;
    let frameCount = 0;
    let windowStart = performance.now();

    const tick = (now: number) => {
      frameCount++;
      const elapsed = now - windowStart;
      if (elapsed >= 1000) {
        const measured = Math.round((frameCount * 1000) / elapsed);
        setFps(measured);
        samplesRef.current += 1;
        setSamples(samplesRef.current);
        frameCount = 0;
        windowStart = now;
      }
      raf = requestAnimationFrame(tick);
    };

    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [enabled]);

  return { fps, samples };
}
