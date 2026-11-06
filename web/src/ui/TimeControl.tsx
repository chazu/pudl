// Pick the moment the graph shows: now (live), an observation snapshot, or
// any time. A past time shows the catalog as pudl had recorded it then.

import { useEffect, useState } from 'react';
import type { GraphSource, SnapshotInfo } from '../data/source';

interface Props {
  source: GraphSource;
  asOf: string | null;
  onAsOf(asOf: string | null): void;
}

const fmt = (iso: string) => new Date(iso).toLocaleString();

/** Value for a datetime-local input, in local time. */
function toLocalInput(iso: string): string {
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

export function TimeControl({ source, asOf, onAsOf }: Props) {
  const [snapshots, setSnapshots] = useState<SnapshotInfo[]>([]);
  const [custom, setCustom] = useState('');
  const [pickedId, setPickedId] = useState<string | null>(null);
  const [picking, setPicking] = useState(false);

  useEffect(() => {
    source.snapshots?.().then(setSnapshots).catch(() => setSnapshots([]));
  }, [source, asOf]);

  // A snapshot's records are written a moment before its row, and facts are
  // timestamped in whole seconds, so viewing "at" a snapshot uses the end of
  // its second (snapshots in the same second look the same).
  const atSnapshot = (s: SnapshotInfo) => new Date(Math.floor(new Date(s.createdAt).getTime() / 1000) * 1000 + 999).toISOString();
  const picked = snapshots.find((s) => s.id === pickedId);
  const selected = !asOf ? 'now' : picked && atSnapshot(picked) === asOf ? picked.id : 'custom';

  // One compact row; the date-time input appears only for a custom time.
  const showCustom = picking || selected === 'custom';
  return (
    <section className="timectl">
      <label className="time-row">
        <span className="time-label">Time</span>
        <select
          value={picking ? 'pick' : selected}
          onChange={(e) => {
            const v = e.target.value;
            setPicking(v === 'pick');
            if (v === 'now') onAsOf(null);
            else if (v !== 'custom' && v !== 'pick') {
              const s = snapshots.find((x) => x.id === v);
              if (s) {
                setPickedId(s.id);
                onAsOf(atSnapshot(s));
              }
            }
          }}
        >
          <option value="now">Now (live)</option>
          {selected === 'custom' && asOf && <option value="custom">{fmt(asOf)}</option>}
          {snapshots.map((s) => (
            <option key={s.id} value={s.id}>
              {fmt(s.createdAt)} · {s.model || s.scope || s.id}
              {s.complete ? '' : ' (incomplete)'}
            </option>
          ))}
          <option value="pick">Custom time…</option>
        </select>
      </label>
      {showCustom && (
        <form
          className="custom-time"
          onSubmit={(e) => {
            e.preventDefault();
            if (custom) {
              setPicking(false);
              onAsOf(new Date(custom).toISOString());
            }
          }}
        >
          <input type="datetime-local" step="1" value={custom || (asOf ? toLocalInput(asOf) : '')} onChange={(e) => setCustom(e.target.value)} />
          <button type="submit" disabled={!custom}>
            Go
          </button>
        </form>
      )}
    </section>
  );
}
