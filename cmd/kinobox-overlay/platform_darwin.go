package main

import "github.com/ebitengine/purego/objc"

// On macOS the window is manipulated through AppKit, which must happen on the
// main thread; SingleThread makes Update run there.
const singleThread = true

const (
	nsApplicationActivationPolicyAccessory = 1 // no Dock icon, no menu bar

	nsWindowCollectionBehaviorCanJoinAllSpaces    = 1 << 0
	nsWindowCollectionBehaviorStationary          = 1 << 4
	nsWindowCollectionBehaviorFullScreenAuxiliary = 1 << 8

	nsStatusWindowLevel = 25
)

// platformSetup lets the strip appear on every Space, including those of
// players in native fullscreen, above normal windows. It returns false until
// the window exists.
func platformSetup() bool {
	sel := objc.RegisterName
	app := objc.ID(objc.GetClass("NSApplication")).Send(sel("sharedApplication"))
	windows := app.Send(sel("windows"))
	n := int(windows.Send(sel("count")))
	if n == 0 {
		return false
	}
	app.Send(sel("setActivationPolicy:"), nsApplicationActivationPolicyAccessory)
	for i := range n {
		w := windows.Send(sel("objectAtIndex:"), i)
		w.Send(sel("setCollectionBehavior:"), nsWindowCollectionBehaviorCanJoinAllSpaces|
			nsWindowCollectionBehaviorStationary|nsWindowCollectionBehaviorFullScreenAuxiliary)
		w.Send(sel("setLevel:"), nsStatusWindowLevel)
	}
	return true
}
