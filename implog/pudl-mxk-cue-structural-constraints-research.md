# pudl-mxk: Option B CUE Schema-Based Wrapper Detection Research

**Date:** 2026-02-18
**Branch:** agent/Moxie/pudl-mxk
**Task:** pudl-mxk — Research robust Option B implementation

## Summary

Investigated whether CUE schema-based collection wrapper detection (Option B from pudl-yqt) can be made robust. Conclusion: **pure CUE cannot express the structural constraint needed**, but a hybrid approach is possible (though not recommended over Option A).

## Key Findings

1. **CUE cannot express "object with at least one unknown-named array-of-objects field"** — pattern constraints (`[string]: T`) apply uniformly to ALL fields, preventing mixed-type structs. No runtime type introspection in comprehensions.

2. **A family of CUE schemas (one per known key name)** is structurally valid but has HIGH false positive risk — cannot incorporate pagination signals, count matching, or attribute blocklists.

3. **TypePattern system can handle structural detection** (GitLab CI precedent), but this is functionally identical to implementing Option A in a different location.

4. **Hybrid (TypePattern + CUE) achieves same quality as Option A** because all intelligence is in Go-side scoring; CUE adds only redundant structural validation.

5. **Recommendation: Option A is strictly better.** CUE extensibility for custom wrapper keys can be added later via simple config, not schema families.

## Research Output

- `.ai/research/pudl-mxk-option-b-cue-schema-wrapper-detection.md` — Full research document with CUE capability analysis, variant exploration, false positive assessment, and concrete recommendation
