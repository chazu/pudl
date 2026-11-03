import { describe, expect, it } from 'vitest';
import { GraphBuilder } from '../data/mock/builder';
import { project, groupAncestors } from '../projection/project';
import { projectionById } from '../projection/registry';
import type { SemanticView } from '../projection/types';
import { initialViewState } from '../visual/viewState';
import { GraphIndex } from './canonical';
import { CATEGORY_IDS, Vocab, type Vocabulary } from './vocabulary';

describe('vocabulary lookups', () => {
  it('fall back safely when nothing is declared', () => {
    const v = new Vocab(undefined);
    expect(v.resourceType('acme.widget.thing')).toMatchObject({ code: 'THIN', label: 'acme.widget.thing', cls: 'external', known: false });
    expect(v.relationshipType('frobs')).toMatchObject({ category: 'other', label: 'frobs' });
    expect(v.categoryOrder()).toEqual([...CATEGORY_IDS]);
    expect(v.containmentTypes()).toEqual([]);
    expect(v.ownerType('acme')).toBeUndefined();
  });

  it('treat empty display info like an undeclared type, but keep its roles', () => {
    const v = new Vocab({
      categories: [],
      edgeTypes: [],
      providers: [],
      resourceTypes: [{ type: 'acme.net', code: '', label: '', class: '', schema: 'acme.#Net', network: { group: true } }],
    });
    expect(v.resourceType('acme.net')).toMatchObject({ code: 'NET', label: 'acme.net', cls: 'external', known: false });
    expect(v.resourceType('acme.net').network?.group).toBe(true);
  });

  it('never upgrade an unknown category, and order categories as declared', () => {
    const v = new Vocab({
      categories: [
        { id: 'iam', label: 'IAM', meaning: 'm', order: 0 },
        { id: 'security', label: 'Sec', meaning: 'm', order: 1 },
        { id: 'made_up', label: 'X', meaning: 'm', order: 2 },
      ],
      edgeTypes: [
        { type: 'grants', category: 'iam', label: 'grants', meaning: 'm' },
        { type: 'weird', category: 'made_up', label: 'weird', meaning: 'm' },
      ],
      providers: [],
      resourceTypes: [],
    });
    expect(v.relationshipType('weird').category).toBe('other');
    expect(v.categoryOrder().slice(0, 2)).toEqual(['iam', 'security']);
    expect(v.categoryOrder()).toHaveLength(CATEGORY_IDS.length);
    expect(v.category('iam').label).toBe('IAM');
  });
});

// A provider no TypeScript knows about: everything it needs is vocabulary.
const ACME: Vocabulary = {
  categories: [{ id: 'containment', label: 'Containment', meaning: 'm', order: 0 }],
  edgeTypes: [
    { type: 'in_zone', category: 'containment', label: 'in zone', meaning: 'm' },
    { type: 'plugged_into', category: 'containment', label: 'plugged into', meaning: 'm' },
  ],
  providers: [{ provider: 'acme', label: 'Acme Cloud' }],
  resourceTypes: [
    { type: 'acme.tenant', code: 'TEN', label: 'Tenant', class: 'admin', admin: { owner: true, parent: [] } },
    { type: 'acme.zone', code: 'ZONE', label: 'Zone', class: 'network', network: { group: true, region: true } },
    { type: 'acme.box', code: 'BOX', label: 'Box', class: 'compute', network: { via: ['in_zone'] }, security: { control: true } },
    { type: 'acme.nic', code: 'NIC', label: 'NIC', class: 'network', network: { via: ['in_zone'] }, portOf: 'plugged_into', inventory: { collapsed: true } },
  ],
};

function acme(): GraphIndex {
  const b = new GraphBuilder('acme');
  b.res({ id: 't', type: 'acme.tenant', name: 'tenant-1', nativeId: 'T1', account: 'T1' });
  b.res({ id: 'z', type: 'acme.zone', name: 'zone-a', account: 'T1', region: 'north' });
  b.res({ id: 'box', type: 'acme.box', name: 'box-1', account: 'T1' });
  b.res({ id: 'lonely', type: 'acme.box', name: 'box-2', account: 'T9' });
  b.res({ id: 'nic', type: 'acme.nic', name: 'nic-1', account: 'T1', region: 'north' });
  b.rel('in_zone', 'box', 'z', 'observed');
  b.rel('in_zone', 'nic', 'z', 'observed');
  b.rel('plugged_into', 'nic', 'box', 'observed');
  return new GraphIndex({ meta: { origin: 'file', label: 't' }, resources: b.resources, relationships: b.relationships, vocabulary: ACME });
}

const path = (view: SemanticView, id: string) => {
  const m = view.members.get(id);
  const start = m?.parentId ?? view.groups.get(view.groupOf.get(id) ?? '')?.parentId ?? null;
  return groupAncestors(view.groups, start)
    .reverse()
    .map((g) => view.groups.get(g)!.label);
};

describe('a new provider declared only in the vocabulary', () => {
  const idx = acme();

  it('is placed in the network view by its roles', () => {
    const net = project(idx, projectionById('network'));
    expect(path(net, 'box')).toEqual(['tenant-1', 'north', 'zone-a']);
    expect(net.members.get('nic')).toMatchObject({ role: 'port', hostId: 'box' });
    // Owner named but not collected: a synthetic group with the owner type's code.
    expect(path(net, 'lonely')).toEqual(['T9']);
    const ghost = [...net.groups.values()].find((g) => g.label === 'T9')!;
    expect(ghost.groupType).toBe('acme.tenant');
  });

  it('groups by owner, region and type in the inventory, collapsing declared buckets', () => {
    const inv = project(idx, projectionById('inventory'));
    expect(path(inv, 'box')).toEqual(['Acme Cloud', 'tenant-1', 'global', 'Box']);
    const collapsed = initialViewState(inv).collapsed;
    const nicBucket = [...inv.groups.values()].find((g) => g.bucketType === 'acme.nic')!;
    expect(collapsed.has(nicBucket.id)).toBe(true);
  });

  it('shows security controls and owner groups in the security view', () => {
    const sec = project(idx, projectionById('security'));
    expect(path(sec, 'box')).toEqual(['tenant-1']);
    expect(sec.groupOf.has('t')).toBe(true);
  });
});
