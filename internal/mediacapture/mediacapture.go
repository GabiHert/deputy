// Package mediacapture lets the window's own page record voice notes: it answers WebKit's
// microphone permission request for Positron's page, which Wails leaves unanswered.
package mediacapture

// Allow makes the webview grant microphone access to Positron's own page (wails:// or the dev
// server on localhost) instead of asking again on every launch. macOS still asks once, with
// NSMicrophoneUsageDescription from Info.plist. Call it before the window is created.
func Allow() { allow() }
