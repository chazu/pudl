// Inspector details: graph_detail rows grouped for display.

import type { Detail } from './canonical';

export interface DetailSection {
  section: string;
  rows: DetailRow[];
}

export interface DetailRow {
  key: string;
  /** Values in rank order. A ranked list is a path; render it as one. */
  values: string[];
  ranked: boolean;
}

/** Groups rows by section and key, keeping the server's order. */
export function groupDetails(details: Detail[] | undefined): DetailSection[] {
  const sections: DetailSection[] = [];
  for (const d of details ?? []) {
    let section = sections.find((s) => s.section === d.section);
    if (!section) {
      section = { section: d.section, rows: [] };
      sections.push(section);
    }
    let row = section.rows.find((r) => r.key === d.key);
    if (!row) {
      row = { key: d.key, values: [], ranked: false };
      section.rows.push(row);
    }
    row.values.push(typeof d.value === 'string' ? d.value : JSON.stringify(d.value));
    if ((d.rank ?? 0) !== 0) row.ranked = true;
  }
  return sections;
}
