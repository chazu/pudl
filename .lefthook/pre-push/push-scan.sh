#!/bin/sh
# Pre-push scan for operational details: blocks a push when an outgoing
# commit's message, file paths or added lines match a term in a local term
# file. The terms are themselves operational details, so the file lives
# outside the repository and is never committed.
#
# Term file: $PUDL_PUSH_SCAN_TERMS, else ~/.config/pudl/push-scan-terms.
# One extended regular expression per line, matched case-insensitively.
# Blank lines and lines starting with # are ignored.
#
# Reads git's pre-push lines on stdin: <local ref> <local sha> <remote ref> <remote sha>.
set -eu

terms=${PUDL_PUSH_SCAN_TERMS:-$HOME/.config/pudl/push-scan-terms}
if [ ! -f "$terms" ]; then
	echo "push-scan: no term file at $terms; create it (one regex per line) before pushing" >&2
	exit 1
fi

patterns=$(mktemp)
found=$(mktemp)
trap 'rm -f "$patterns" "$found"' EXIT
grep -v -E '^[[:space:]]*(#|$)' "$terms" >"$patterns" || true
if [ ! -s "$patterns" ]; then
	echo "push-scan: $terms has no terms; add at least one before pushing" >&2
	exit 1
fi

zero=0000000000000000000000000000000000000000
while read -r _local_ref local_sha _remote_ref remote_sha; do
	case $local_sha in "$zero" | "") continue ;; esac
	if [ "$remote_sha" != "$zero" ] && git cat-file -e "$remote_sha^{commit}" 2>/dev/null; then
		commits=$(git rev-list "$remote_sha..$local_sha")
	else
		commits=$(git rev-list "$local_sha" --not --remotes)
	fi
	for c in $commits; do
		# The message, then every added line of the patch (+++ lines carry
		# the new file paths).
		{
			git log -1 --format=%B "$c"
			git show --format= --patch --no-color "$c" | grep '^+' || true
		} | grep -n -i -E -f "$patterns" | sed "s/^/$(git rev-parse --short "$c"): /" >>"$found" || true
	done
done

if [ -s "$found" ]; then
	echo "push-scan: outgoing commits contain operational details; remove them and rewrite the commits:" >&2
	cut -c1-200 "$found" >&2
	exit 1
fi
