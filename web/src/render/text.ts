// Fitting labels into a fixed width.

/**
 * Shorten `s` to at most `max` characters by cutting from the middle. Cloud
 * resource names often share long prefixes (gke-<cluster>-<hash>-...), so the
 * end usually tells similar names apart and is kept a little longer.
 */
export function middleEllipsis(s: string, max: number): string {
  if (s.length <= max) return s;
  if (max <= 1) return '…';
  const keep = max - 1;
  const head = Math.floor(keep * 0.45);
  return `${s.slice(0, head)}…${s.slice(s.length - (keep - head))}`;
}
