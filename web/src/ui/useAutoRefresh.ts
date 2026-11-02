// Reload when the catalog changes (for example when a pudl check finishes in
// another terminal). Live sources push the change stamp (/api/events); if
// the stream is unavailable this falls back to polling version() every
// AUTO_REFRESH_MS. A change seen while the tab is hidden reloads when it is
// shown again. Refs keep one subscription across reloads.

import { useEffect, useRef } from 'react';
import type { GraphSource } from '../data/source';

export const AUTO_REFRESH_MS = 3000;

export function useAutoRefresh(
  source: GraphSource | null,
  loadedVersion: string | undefined,
  enabled: boolean,
  reloading: boolean,
  reload: () => void,
) {
  const loaded = useRef(loadedVersion);
  const busy = useRef(reloading);
  const doReload = useRef(reload);
  loaded.current = loadedVersion;
  busy.current = reloading;
  doReload.current = reload;

  useEffect(() => {
    if (!enabled || !source) return;
    let pending = false;
    const seen = (version: string | null) => {
      if (!version || !loaded.current || version === loaded.current || busy.current) return;
      if (document.hidden) pending = true;
      else doReload.current();
    };
    const onVisible = () => {
      if (!document.hidden && pending) {
        pending = false;
        doReload.current();
      }
    };
    document.addEventListener('visibilitychange', onVisible);

    let timer: ReturnType<typeof setInterval> | null = null;
    const poll = () => {
      if (timer || !source.version) return;
      timer = setInterval(() => {
        if (!document.hidden) source.version!().then(seen).catch(() => {});
      }, AUTO_REFRESH_MS);
    };
    const unsubscribe = source.subscribe ? source.subscribe(seen, poll) : (poll(), () => {});
    return () => {
      unsubscribe();
      if (timer) clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [source, enabled]);
}
