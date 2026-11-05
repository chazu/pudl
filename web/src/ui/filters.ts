// "Only" filters for the sidebar's scope and resource-type lists: isolate one
// item in a click, and a second click on the same item shows everything again.

/** Whether owner is the only one in scope (null scope: every owner). */
export function isOnlyOwner(scope: ReadonlySet<string> | null, owner: string): boolean {
  return scope !== null && scope.size === 1 && scope.has(owner);
}

/** Scope after "only" on owner: just that owner, or every owner when it already was. */
export function onlyOwner(scope: ReadonlySet<string> | null, owner: string): ReadonlySet<string> | null {
  return isOnlyOwner(scope, owner) ? null : new Set([owner]);
}

/** Whether type is the only one of allTypes left visible. */
export function isOnlyType(hidden: ReadonlySet<string>, type: string, allTypes: Iterable<string>): boolean {
  if (hidden.has(type)) return false;
  for (const t of allTypes) {
    if (t !== type && !hidden.has(t)) return false;
  }
  return true;
}

/**
 * Hidden types after "only" on type: every other type of allTypes (the whole
 * graph's, so the choice holds in every view), or none when type already was
 * the only one shown.
 */
export function onlyType(hidden: ReadonlySet<string>, type: string, allTypes: Iterable<string>): ReadonlySet<string> {
  const all = [...allTypes];
  if (isOnlyType(hidden, type, all)) return new Set();
  return new Set(all.filter((t) => t !== type));
}
