// SVG shapes for visual nodes and ports. Stateless; positions come from the
// layout result.

import type { Box } from '../layout/types';
import type { VisualNode, VisualPort } from '../visual/types';
import { middleEllipsis } from './text';

const truncate = middleEllipsis;
const charsFor = (width: number, px: number) => Math.max(4, Math.floor(width / (px * 0.58)));

interface NodeProps {
  node: VisualNode;
  box: Box;
  dimmed: boolean;
  selected: boolean;
  depth: number;
  onSelect: (node: VisualNode) => void;
  onToggle: (node: VisualNode) => void;
  onExpand?: (node: VisualNode) => void;
}

/** "⋯N": relationships to resources not loaded yet (partial graphs). Click loads them. */
/** Width of the not-loaded badge drawn at a node's right edge (0 when none). */
function badgeWidth(node: VisualNode): number {
  return node.notLoaded > 0 ? `⋯${node.notLoaded}`.length * 6.2 + 8 : 0;
}

function NotLoadedBadge({ node, x, y, onExpand }: { node: VisualNode; x: number; y: number; onExpand?: (n: VisualNode) => void }) {
  if (node.notLoaded <= 0) return null;
  const label = `⋯${node.notLoaded}`;
  return (
    <g
      className="notloaded"
      transform={`translate(${x},${y})`}
      onClick={(e) => {
        e.stopPropagation();
        onExpand?.(node);
      }}
    >
      <rect x={-label.length * 6.2 - 6} y={-9} width={label.length * 6.2 + 8} height={13} rx={6} />
      <text x={-3} y={1} textAnchor="end">{label}</text>
      <title>{`${node.notLoaded} relationship(s) to resources not loaded yet. Click to load neighbours.`}</title>
    </g>
  );
}

export function GroupShape({ node, box, selected, depth, onSelect, onToggle, onExpand }: NodeProps) {
  const header = 26;
  const labelRoom = box.width - 70 - node.code.length * 7 - badgeWidth(node);
  const label = truncate(node.label, charsFor(labelRoom, 12));
  // The sublabel takes whatever header width the label leaves, if enough to read.
  const subChars = Math.floor((labelRoom - label.length * 12 * 0.58 - 12) / (10.5 * 0.58));
  return (
    <g className={`vgroup depth-${depth % 4}${node.synthetic ? ' synthetic' : ''}${selected ? ' selected' : ''}`} transform={`translate(${box.x},${box.y})`}>
      <rect className="vgroup-body" width={box.width} height={box.height} rx={4} onClick={(e) => { e.stopPropagation(); onSelect(node); }} />
      <rect className="vgroup-header" width={box.width} height={header} rx={4} onClick={(e) => { e.stopPropagation(); onSelect(node); }} onDoubleClick={(e) => { e.stopPropagation(); onToggle(node); }} />
      <g className="toggle" onClick={(e) => { e.stopPropagation(); onToggle(node); }}>
        <rect x={4} y={5} width={16} height={16} rx={2} />
        <text x={12} y={17} textAnchor="middle">−</text>
      </g>
      <text className={`code cls-${node.styleType}`} x={26} y={17}>{node.code}</text>
      <text className="glabel" x={26 + node.code.length * 7 + 8} y={17}>
        {label}
        {node.sublabel && subChars >= 8 ? <tspan className="gsub">{`  ${truncate(node.sublabel, subChars)}`}</tspan> : null}
      </text>
      <title>{`${node.synthetic ? 'Grouping (not a resource)' : 'Resource group'}: ${node.label}. Double-click header to collapse.`}</title>
      <NotLoadedBadge node={node} x={box.width - 4} y={16} onExpand={onExpand} />
    </g>
  );
}

export function CollapsedShape({ node, box, dimmed, selected, onSelect, onToggle, onExpand }: NodeProps) {
  const internal = node.internalRelationships?.length ?? 0;
  return (
    <g className={`vcollapsed${node.synthetic ? ' synthetic' : ''}${dimmed ? ' dimmed' : ''}${selected ? ' selected' : ''}`} transform={`translate(${box.x},${box.y})`} onClick={(e) => { e.stopPropagation(); onSelect(node); }} onDoubleClick={(e) => { e.stopPropagation(); onToggle(node); }}>
      <rect className="outer" width={box.width} height={box.height} rx={4} />
      <rect className="inner" x={3} y={3} width={box.width - 6} height={box.height - 6} rx={3} />
      <g className="toggle" onClick={(e) => { e.stopPropagation(); onToggle(node); }}>
        <rect x={8} y={8} width={16} height={16} rx={2} />
        <text x={16} y={20} textAnchor="middle">+</text>
      </g>
      <text className={`code cls-${node.styleType}`} x={30} y={20}>{node.code}</text>
      <text className="nlabel" x={30 + node.code.length * 7 + 6} y={20}>{truncate(node.label, charsFor(box.width - 60 - node.code.length * 7, 12))}</text>
      <text className="nsub" x={10} y={42}>{`${node.memberCount ?? 0} resources${internal ? ` · ${internal} internal rel.` : ''}`}</text>
      <title>{`Collapsed: ${node.label}. Edges to its contents end at its boundary. Double-click to expand.`}</title>
      <NotLoadedBadge node={node} x={box.width - 4} y={box.height - 8} onExpand={onExpand} />
    </g>
  );
}

export function LeafShape({ node, box, dimmed, selected, onSelect, onExpand }: NodeProps) {
  const unresolved = node.kind === 'unresolved';
  return (
    <g className={`vnode cls-${node.styleType}${unresolved ? ' unresolved' : ''}${dimmed ? ' dimmed' : ''}${selected ? ' selected' : ''}`} transform={`translate(${box.x},${box.y})`} onClick={(e) => { e.stopPropagation(); onSelect(node); }}>
      <rect className="body" width={box.width} height={box.height} rx={3} />
      {!unresolved && <rect className="stripe" width={4} height={box.height} rx={1} />}
      <text className={`code cls-${node.styleType}`} x={11} y={17}>{node.code}</text>
      <text className="nlabel" x={11 + Math.max(3, node.code.length) * 7 + 6} y={17}>{truncate(node.label, charsFor(box.width - 30 - node.code.length * 7 - (node.undrawnCount > 0 ? 18 : 0), 12))}</text>
      {node.sublabel && <text className="nsub" x={11} y={34}>{truncate(node.sublabel, charsFor(box.width - 22, 10.5))}</text>}
      {node.undrawnCount > 0 && (
        <g className="badge" transform={`translate(${box.width - 5},13)`}>
          <text textAnchor="end">{`+${node.undrawnCount}`}</text>
          <title>{`${node.undrawnCount} relationship(s) hidden by current filters`}</title>
        </g>
      )}
      <NotLoadedBadge node={node} x={box.width - 3} y={box.height - 6} onExpand={onExpand} />
    </g>
  );
}

export function PortShape({ port, box, dimmed, onSelect }: { port: VisualPort; box: Box; dimmed: boolean; onSelect: (p: VisualPort) => void }) {
  const cx = box.x + box.width / 2;
  const cy = box.y + box.height / 2;
  if (port.kind === 'semantic') {
    return (
      <g className={`vport semantic${dimmed ? ' dimmed' : ''}`} onClick={(e) => { e.stopPropagation(); onSelect(port); }}>
        <circle cx={cx} cy={cy} r={box.width / 2} />
        <text x={cx} y={cy + 15} textAnchor="middle">{port.label?.split(' ').pop()}</text>
        <title>{`Interface ${port.label ?? ''} (a real network interface)`}</title>
      </g>
    );
  }
  if (port.kind === 'boundary') {
    const r = box.width / 2 + 1;
    return (
      <g className={`vport boundary${dimmed ? ' dimmed' : ''}`}>
        <path d={`M${cx},${cy - r} L${cx + r},${cy} L${cx},${cy + r} L${cx - r},${cy} Z`} />
        <title>Generated boundary port: edges to resources inside this collapsed group end here. Not a network hop.</title>
      </g>
    );
  }
  return <rect className={`vport logical cat-${port.category}${dimmed ? ' dimmed' : ''}`} x={box.x + 1} y={box.y + 1} width={box.width - 2} height={box.height - 2} />;
}
