import CoreGraphics
// A zero-distance mouse move at the HID level: input as far as idle
// detection is concerned, with nothing on screen changing.
let src = CGEventSource(stateID: .hidSystemState)
let at = CGEvent(source: nil)?.location ?? .zero
CGEvent(mouseEventSource: src, mouseType: .mouseMoved, mouseCursorPosition: at, mouseButton: .left)?.post(tap: .cghidEventTap)
