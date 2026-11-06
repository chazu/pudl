# Pre-push scan for operational details (2026-11-06)

## Problem

The repository is public. A design commit named an organization's repo,
projects and how it runs its infrastructure, and was pushed before anyone
noticed. Gitleaks finds secrets, not operational details.

## What changed

1. **`scripts/push-scan.sh`**, run by a new lefthook `pre-push` job
   (`use_stdin: true`). For each outgoing commit it scans the message and
   every added patch line (`+++` lines carry new file paths) against a term
   file, case-insensitively. Any match blocks the push and lists the commit
   and line. A missing or empty term file blocks the push too.
2. **Term file outside the repo**: `$PUDL_PUSH_SCAN_TERMS`, else
   `~/.config/pudl/push-scan-terms`. One extended regex per line; `#`
   comments and blank lines are ignored. The terms are operational details
   themselves, so they are never committed.
3. **`CLAUDE.md` hard rule**: review each outgoing commit for operational
   details before every push, add new terms to the local file, never skip
   the hook.

## Range rules

- Normal push: `<remote sha>..<local sha>`.
- New branch, or a remote sha that is not local (another clone pushed):
  every commit of the local sha that no remote-tracking ref has.
- Branch deletes are skipped.

## Setup per clone

`lefthook install`, then create the term file.
