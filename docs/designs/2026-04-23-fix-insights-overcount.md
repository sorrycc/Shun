# Fix Insights Over-Count — Design Document

**Date:** 2026-04-23
**Status:** Proposed

## 1. Background

Users report the Insights tab shows hotkey usage counts "much more than I used." Inspection of `~/Library/Application Support/Shun/usage.json` confirms it: of 4167 total events, 2453 (59%) are duplicates in clusters of 12–60 events sharing the same ISO-second + bundle identifier. Cluster sizes match macOS keyboard autorepeat rates (12–15/sec).

## 2. Requirements Summary

**Goal:** Stop over-counting hotkey usage caused by keyboard autorepeat bursts, and clean up already-inflated history.

**Scope:**
- In scope: Dedup at `UsageStore.log` for new events; dedup historical `usage.json` on load.
- Out of scope: Root-cause fix of the CGEvent tap's autorepeat filter in `ShortcutManager.swift:255`. Activate and launch are idempotent, so repeated calls have no visible side effect beyond the inflated count we are fixing here.

## 3. Acceptance Criteria

1. Holding a hotkey for ≥ 1 sec produces exactly 1 log event, not 15.
2. On next app launch, historical duplicates in `usage.json` collapse on load (measured: 4167 → ~1928 events in the current file).
3. Legitimate press patterns (activate → switch to another app → re-activate, > 500 ms apart) still count correctly.
4. Both CGEvent tap and Carbon fallback paths benefit from a single fix (both call `UsageStore.log`).

## 4. Problem Analysis

Evidence from `usage.json` (histogram of same-second + same-bundle cluster sizes):

- size 1: 1714 clusters (legitimate singletons)
- size 12: 38 clusters
- size 13: 99 clusters
- size 15: 29 clusters
- size 30: 2 clusters
- size 60: 1 cluster

Sizes and intra-cluster gaps match macOS key-repeat behavior. The tap's autorepeat filter (`event.getIntegerValueField(.keyboardEventAutorepeat) != 0` in `ShortcutManager.swift:255`) is not catching them in practice — possibly because the autorepeat field is unreliable at `.cgSessionEventTap` for some hotkey combinations.

Approaches evaluated:

- **A — Fix autorepeat filter at tap layer**: harder to diagnose without repro; touches the hot path; risky.
- **B — Dedup in `UsageStore.log`**: small, single-site change; both activation paths already funnel through `log()`; idempotent activations still work.
- **Chosen: B** — minimal, robust, addresses the visible bug; historical cleanup is free via load-time dedup.

## 5. Decision Log

**1. Where to dedup?**
- Options: A) CGEvent tap · B) `UsageStore.log` · C) Both
- Decision: **B)** — Single chokepoint covers both activation paths; keeps tap hot path simple.

**2. Dedup window length?**
- Options: A) 250 ms · B) 500 ms · C) 1000 ms
- Decision: **B) 500 ms** — Covers autorepeat rates ≥ 2 Hz (faster than every realistic OS setting). Allows 2 distinct presses/sec, faster than any plausible toggle cycle.

**3. Dedup key?**
- Options: A) Compare against `events.last` · B) Map of bundle → last timestamp
- Decision: **A)** — Autorepeat bursts are always contiguous in event order, so `events.last` is sufficient. No extra state.

**4. Historical cleanup?**
- Options: A) Skip · B) Dedup on load · C) Migration script
- Decision: **B)** — Apply the same dedup routine after the existing 7-day prune in `load()`.

## 6. Design

Add a single private helper and reuse it in two places:

```swift
private static let dedupWindow: TimeInterval = 0.5

func log(bundleIdentifier: String) {
    let now = Date()
    if let last = events.last,
       last.bundleIdentifier == bundleIdentifier,
       now.timeIntervalSince(last.timestamp) < Self.dedupWindow {
        return
    }
    events.append(UsageEvent(bundleIdentifier: bundleIdentifier, timestamp: now))
    save()
}

private func dedupAutorepeatBursts() {
    var result: [UsageEvent] = []
    result.reserveCapacity(events.count)
    for event in events {
        if let last = result.last,
           last.bundleIdentifier == event.bundleIdentifier,
           event.timestamp.timeIntervalSince(last.timestamp) < Self.dedupWindow {
            continue
        }
        result.append(event)
    }
    events = result
}
```

Invoke `dedupAutorepeatBursts()` after `pruneOldEvents()` in `load()`. This collapses existing bursts in-memory on load; the next debounced save rewrites `usage.json` in cleaned form.

Correctness notes:
- `events` are appended in chronological order (only `log()` mutates them), so `events.last` comparison is a valid reducer for the load-time pass.
- Log-time dedup returns early without calling `save()`, so disk I/O is unchanged for deduped events.
- `Date()` is called once in `log()` to avoid a microsecond skew between the comparison and the appended timestamp.

## 7. Files Changed

- `Sources/Shun/Models/UsageStore.swift` — add `dedupWindow` constant, early-return in `log()`, and `dedupAutorepeatBursts()` pass in `load()`.

## 8. Verification

1. [AC1] Manual: focus a different app, hold a shortcut for ≥ 1 sec, open Insights → count increments by 1.
2. [AC2] Relaunch the app → `usage.json` size drops and Insights total matches reality. Confirm by rerunning the dedup-count Python analysis used during diagnosis.
3. [AC3] Activate A → click B to bring it frontmost → press A's hotkey again (>500 ms later) → count increments by 2 total.
4. [AC4] Code check: `UsageStore.log` is the single logging entry point; both tap and Carbon paths use it.
5. `swift build` passes.
