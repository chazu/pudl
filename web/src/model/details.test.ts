import { describe, expect, it } from 'vitest';
import { groupDetails } from './details';

describe('groupDetails', () => {
  it('groups rows by section and key in the given order', () => {
    const out = groupDetails([
      { section: 'Pulumi', key: 'component', value: 'root', rank: 0 },
      { section: 'Pulumi', key: 'component', value: 'network', rank: 1 },
      { section: 'Pulumi', key: 'stack', value: 'org/proj/prod' },
      { section: 'Audit', key: 'ports', value: 443 },
    ]);
    expect(out).toEqual([
      {
        section: 'Pulumi',
        rows: [
          { key: 'component', values: ['root', 'network'], ranked: true },
          { key: 'stack', values: ['org/proj/prod'], ranked: false },
        ],
      },
      { section: 'Audit', rows: [{ key: 'ports', values: ['443'], ranked: false }] },
    ]);
  });

  it('is empty without details', () => {
    expect(groupDetails(undefined)).toEqual([]);
  });
});
