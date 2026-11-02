// The current projection's grouping as a browsable tree. Mirrors the
// canvas: the same semantic groups and the same collapse state.

import { useMemo, useState } from 'react';
import type { ResourceId } from '../model/canonical';
import type { SemanticView } from '../projection/types';
import type { Selection } from '../render/highlight';
import type { ViewState } from '../visual/viewState';

interface Props {
  view: SemanticView;
  state: ViewState;
  selection: Selection;
  nameOf(id: ResourceId): string;
  onSelectResource(id: ResourceId): void;
  onSelectGroup(id: string): void;
  onToggleGroup(id: string): void;
}

interface TreeItem {
  id: string;
  label: string;
  groupId?: string;
  resourceId?: ResourceId;
  children: TreeItem[];
}

export function HierarchyTree({ view, state, selection, nameOf, onSelectResource, onSelectGroup, onToggleGroup }: Props) {
  const [openLeaves, setOpenLeaves] = useState<Set<string>>(new Set());
  const roots = useMemo(() => buildTree(view, nameOf), [view, nameOf]);
  const selectedId = selection?.kind === 'resource' ? selection.id : selection?.kind === 'group' ? selection.id : null;

  const render = (item: TreeItem, depth: number) => {
    const isGroup = !!item.groupId;
    const collapsed = isGroup && state.collapsed.has(item.groupId!);
    const leavesShown = openLeaves.has(item.id);
    const leafCount = item.children.filter((c) => !c.groupId).length;
    const active = selectedId === item.resourceId || selectedId === item.groupId;
    return (
      <li key={item.id}>
        <div className={`tree-row${active ? ' active' : ''}`} style={{ paddingLeft: depth * 12 }}>
          {isGroup ? (
            <button className="twisty" title={collapsed ? 'Expand on canvas' : 'Collapse on canvas'} onClick={() => onToggleGroup(item.groupId!)}>
              {collapsed ? '▸' : '▾'}
            </button>
          ) : (
            <span className="twisty" />
          )}
          <span
            className={isGroup ? 'tree-group' : 'tree-leaf'}
            onClick={() => (item.resourceId ? onSelectResource(item.resourceId) : item.groupId && onSelectGroup(item.groupId))}
          >
            {item.label}
          </span>
          {isGroup && leafCount > 0 && (
            <button className="leafcount" title="Show resources in tree" onClick={() => setOpenLeaves((s) => toggle(s, item.id))}>
              {leafCount}
            </button>
          )}
        </div>
        {isGroup && item.children.length > 0 && (
          <ul>{item.children.filter((c) => c.groupId || leavesShown).map((c) => render(c, depth + 1))}</ul>
        )}
      </li>
    );
  };
  return <ul className="tree">{roots.map((r) => render(r, 0))}</ul>;
}

function toggle(s: Set<string>, id: string): Set<string> {
  const next = new Set(s);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  return next;
}

function buildTree(view: SemanticView, nameOf: (id: ResourceId) => string): TreeItem[] {
  const items = new Map<string, TreeItem>();
  const roots: TreeItem[] = [];
  const groups = [...view.groups.values()].sort((a, b) => a.order - b.order);
  for (const g of groups) items.set(g.id, { id: g.id, label: g.label, groupId: g.id, resourceId: g.resourceId, children: [] });
  for (const g of groups) {
    const item = items.get(g.id)!;
    const parent = g.parentId ? items.get(g.parentId) : undefined;
    (parent ? parent.children : roots).push(item);
  }
  const members = [...view.members.values()].filter((m) => m.role !== 'port').sort((a, b) => a.order - b.order);
  for (const m of members) {
    const item: TreeItem = { id: m.resourceId, label: nameOf(m.resourceId), resourceId: m.resourceId, children: [] };
    const parent = m.parentId ? items.get(m.parentId) : undefined;
    (parent ? parent.children : roots).push(item);
  }
  return roots;
}
