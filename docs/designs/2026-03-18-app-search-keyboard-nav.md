# App Search Keyboard Navigation

## Decision Log

**1. Where to handle keyboard navigation?**
- Options: A) `.onKeyPress` on parent VStack · B) Custom NSViewRepresentable · C) List selection binding
- Decision: **A) `.onKeyPress`** — macOS 14+ native, intercepts up/down while TextField retains focus

**2. Selection tracking?**
- Options: A) Index-based (`Int`) · B) ID-based (`String?`)
- Decision: **A) Index-based** — simpler increment/decrement

**3. Visual highlight?**
- Options: A) `.listRowBackground` · B) List selection binding · C) Custom overlay
- Decision: **A) `.listRowBackground`** — works with TextField focused

**4. Return key?**
- Reuse `.onSubmit`, add app at `selectedIndex`

## Changes

File: `Sources/HotApp/Views/AppSearchView.swift`

- Add `@State private var selectedIndex = 0`
- Add `.onKeyPress(.upArrow)` / `.onKeyPress(.downArrow)` on outer VStack
- Reset selectedIndex on search text change (clamped)
- Wrap list in `ScrollViewReader`, auto-scroll on selectedIndex change
- Apply `.listRowBackground` highlight to selected row
- Change `.onSubmit` to use selectedIndex
