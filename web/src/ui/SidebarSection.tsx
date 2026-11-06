// A sidebar section whose heading folds it. The folded state is a per-viewer
// convenience kept in localStorage (best effort). A summary beside the
// heading keeps a folded section's state visible (e.g. an active filter).

import { useState, type ReactNode } from 'react';

const KEY = 'pudl.sidebar.folded';

function readFolded(): Record<string, boolean> {
  try {
    return JSON.parse(localStorage.getItem(KEY) ?? '{}') ?? {};
  } catch {
    return {};
  }
}

function writeFolded(id: string, folded: boolean) {
  try {
    localStorage.setItem(KEY, JSON.stringify({ ...readFolded(), [id]: folded }));
  } catch {
    // Storage unavailable: folding still works for this page.
  }
}

interface Props {
  /** Stable key for remembering the folded state. */
  id: string;
  title: ReactNode;
  /** Shown beside the title, folded or not. */
  summary?: ReactNode;
  /** Highlight the summary (e.g. a filter is active). */
  summaryActive?: boolean;
  defaultFolded?: boolean;
  /** Take the sidebar's remaining height. */
  grow?: boolean;
  children: ReactNode;
}

export function SidebarSection({ id, title, summary, summaryActive, defaultFolded = false, grow, children }: Props) {
  const [folded, setFolded] = useState(() => readFolded()[id] ?? defaultFolded);
  const toggle = () => {
    setFolded(!folded);
    writeFolded(id, !folded);
  };
  return (
    <section className={`${grow && !folded ? 'grow' : ''}${folded ? ' folded' : ''}`}>
      <h3>
        <button type="button" className="fold" aria-expanded={!folded} onClick={toggle}>
          <span className="twist">{folded ? '▸' : '▾'}</span>
          {title}
        </button>
        {summary !== undefined && <span className={`sec-summary${summaryActive ? ' active' : ''}`}>{summary}</span>}
      </h3>
      {!folded && children}
    </section>
  );
}
