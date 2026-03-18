# Fix: Accessibility Permission Prompted on Every Launch

## Problem
`AXIsProcessTrustedWithOptions` called with `AXTrustedCheckOptionPrompt: true` triggers the macOS system accessibility prompt on every app launch, even when permission is already granted.

## Decision Log

**1. How to check accessibility without prompting?**
- Options: A) `AXIsProcessTrusted()` (no options) · B) `AXIsProcessTrustedWithOptions` with `prompt: false`
- Decision: **B) `prompt: false`** — Same API, explicitly documents intent

**2. When to show the system prompt?**
- Options: A) Never (rely on custom NSAlert) · B) Show system prompt only when not trusted
- Decision: **A) Never** — Custom alert already provides better UX with explanation and "Open System Settings" button

## Fix
Change `"AXTrustedCheckOptionPrompt": true` to `false` in `AppDelegate.swift:76`. The custom NSAlert already handles the untrusted case.
