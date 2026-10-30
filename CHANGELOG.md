# Changelog

All notable changes to Shun are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Releases follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 1.3.0
- Shun no longer sends a daily usage ping, and **Settings → Privacy** drops its switch.
- Crash reports no longer include an anonymous ID, and the **Anonymous ID** row is gone.
- Sign in to mon.sorrycc.dev in the new **Settings → Account** tab, approving a code in the browser.
- Signed in, your shortcuts and **Enable All Shortcuts** sync between your Macs.
- Shortcuts changed on two Macs merge app by app, and a shortcut of an app not installed shows as missing.
- The first sync of a Mac with shortcuts of its own asks whether to keep this Mac's or the account's.
- The **Account** tab shows when Shun last synced, with **Sync Now**, **Manage Account** and **Sign Out**.
- The menu bar menu gains **Sign In to Sync…**, which shows the account's name once signed in.
- Crash reports now go to Mon, anonymously and with your home folder taken out.
- Filter the **Shortcuts** list by app name or bundle ID with the new search field; `⌘F` focuses it.

## 1.2.0
- **Settings** is redesigned, with icon tabs in the title bar and settings grouped in cards.
- `⌘1`–`⌘4` switch between the Settings tabs, and the tabs' tooltips show these keys.
- Shortcuts show their keys as keycaps, in a soft well instead of a bordered box.
- While recording, the recorder shows the modifiers you hold and tells you what to press.
- A key pressed without a modifier shakes the recorder instead of doing nothing.
- Removing a shortcut shows **Undo** instead of asking first.
- Right-click a shortcut to show its app in Finder, turn it off, clear its keys or remove it.
- Shortcut rows highlight on hover and show a drag handle; the selected row gets a soft highlight.
- The shortcut list shows how many apps it has, and fades the switch of an app that is missing.
- Under a short list, a hint says you can drag apps in from Finder.
- **Shortcuts** shows a banner with **Resume** while shortcuts are paused, and dims the list.
- **Shortcuts** shows a banner while Shun lacks the Accessibility permission, with a button to grant it.
- **General** shows whether Shun has the Accessibility permission, with a button to grant it.
- **General** ends with Shun's mark and version.
- **Insights** shows totals for the week and today, and a chart of the last 7 days.
- **Insights** shows your daily average, compares today with it, and draws it across the chart.
- **Insights** shows each app's share of your switches.
- **Privacy** explains what stays on your Mac even when telemetry is off.
- The app picker ranks names that start with what you type first, and shows keyboard hints.
- The app picker shows placeholder rows while it scans your apps, and a button to clear the search.
- The menu bar menu lists your shortcuts with their keys; choose one to bring its app forward.
- Settings animates switching tabs, recording keys and adding apps, and chart bars highlight on hover.
- With **Reduce Motion** on, the recorder and the app picker stop animating while they wait.
- Lists scroll smoothly, and reopening Settings shows your apps' icons at once.
- Settings uses less CPU, and no longer redraws while hidden.
- The Settings window is a little taller, so **Insights** needs less scrolling.

## 1.1.0
- Shun is rebuilt as one small native app that uses less memory in the menu bar.
- Your shortcuts, usage history and privacy choices carry over, and you can still go back to 1.0.x.
- Updates come from Shun's GitHub releases, in a new update window, and download only what changed.
- Granting the Accessibility permission takes effect without relaunching Shun.
- Until you grant it, shortcuts still work, more slowly.
- Fix **Import Recipe** counting shortcuts with duplicate keys twice in its summary.

## 1.0.10 - 2026-06-29
- On macOS 26, **Settings** opens reliably and comes to the front.

## 1.0.9 - 2026-04-23
- Fix **Insights** counting a held shortcut once per key repeat; inflated history is corrected.

## 1.0.8 - 2026-04-22
- A new **Privacy** tab turns off anonymous crash reports and a daily usage ping, both on by default.
- Telemetry sends no personal data and no keystrokes.

## 1.0.7 - 2026-04-22
- Updates come from `download.sorrycc.dev`; installed versions keep updating from the old host.
- **Check for Updates…** brings the update window to the front.

## 1.0.6 - 2026-04-22
- Shortcuts reach the remote Mac instead of Shun while Screen Sharing is frontmost.

## 1.0.5 - 2026-04-22
- Fix a crash on recording a shortcut.

## 1.0.4 - 2026-04-20
- Shun launches again on clean machines; if your version crashes on launch, download this one by hand.

## 1.0.3 - 2026-04-18
- Fix a crash at launch in 1.0.2; if you're on 1.0.1, download this version by hand.

## 1.0.2 - 2026-04-18
- Shun checks for updates once a day and installs them in one click.
- **Check for Updates…** is in the menu bar menu and in **Settings → General**.
- Update checks send no system profile data.

## 1.0.1 - 2026-04-16
- A new app icon.
- A new menu bar icon, sharp on every display.
- Shun is signed and notarized, so macOS opens it without warnings.
