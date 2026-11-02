// Poll a live source's change stamp and reload when the catalog changed
// (for example when a pudl check finishes in another terminal). Paused while
// the tab is hidden or a reload is running.

import { useEffect, useRef } from 'react';
import type { GraphSource } from '../data/source';

export const AUTO_REFRESH_MS = 3000;

export function useAutoRefresh(
  source: GraphSource | null,
  loadedVersion: string | undefined,
  enabled: boolean,
  reload: () => void,
) {
  const busy = useRef(false);
  useEffect(() => {
    if (!enabled || !source?.version || !loadedVersion) return;
    const timer = setInterval(async () => {
      if (busy.current || document.hidden) return;
      busy.current = true;
      try {
        const current = await source.version!();
        if (current && current !== loadedVersion) reload();
      } finally {
        busy.current = false;
      }
    }, AUTO_REFRESH_MS);
    return () => clearInterval(timer);
  }, [source, loadedVersion, enabled, reload]);
}
