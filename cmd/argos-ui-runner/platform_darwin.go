//go:build darwin

package main

/*
#cgo LDFLAGS: -framework ApplicationServices
#include <ApplicationServices/ApplicationServices.h>

static bool meshclawAXTrusted(bool prompt) {
	if (!prompt) {
		return AXIsProcessTrusted();
	}
	const void *keys[] = { kAXTrustedCheckOptionPrompt };
	const void *values[] = { kCFBooleanTrue };
	CFDictionaryRef options = CFDictionaryCreate(
		kCFAllocatorDefault,
		keys,
		values,
		1,
		&kCFCopyStringDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	bool trusted = AXIsProcessTrustedWithOptions(options);
	CFRelease(options);
	return trusted;
}

static void meshclawClick(double x, double y) {
	CGPoint point = CGPointMake(x, y);
	CGEventRef move = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, point, kCGMouseButtonLeft);
	CGEventRef down = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, point, kCGMouseButtonLeft);
	CGEventRef up = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, point, kCGMouseButtonLeft);
	CGEventPost(kCGHIDEventTap, move);
	CGEventPost(kCGHIDEventTap, down);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(move);
	CFRelease(down);
	CFRelease(up);
}

static void meshclawPostKey(CGKeyCode key, CGEventFlags flags) {
	CGEventRef down = CGEventCreateKeyboardEvent(NULL, key, true);
	CGEventRef up = CGEventCreateKeyboardEvent(NULL, key, false);
	CGEventSetFlags(down, flags);
	CGEventSetFlags(up, flags);
	CGEventPost(kCGHIDEventTap, down);
	CGEventPost(kCGHIDEventTap, up);
	CFRelease(down);
	CFRelease(up);
}

static void meshclawTypeUnicode(const UniChar *chars, long length) {
	for (long i = 0; i < length; i++) {
		UniChar c = chars[i];
		CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
		CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
		CGEventKeyboardSetUnicodeString(down, 1, &c);
		CGEventKeyboardSetUnicodeString(up, 1, &c);
		CGEventPost(kCGHIDEventTap, down);
		CGEventPost(kCGHIDEventTap, up);
		CFRelease(down);
		CFRelease(up);
	}
}
*/
import "C"

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unsafe"
)

func accessibilityTrusted(prompt bool) bool {
	return bool(C.meshclawAXTrusted(C.bool(prompt)))
}

func clickPoint(x, y float64) error {
	C.meshclawClick(C.double(x), C.double(y))
	return nil
}

func pressKey(key string, modifiers []string) error {
	code, ok := keyCodes[strings.ToLower(strings.TrimSpace(key))]
	if !ok {
		return fmt.Errorf("unsupported key %q", key)
	}
	C.meshclawPostKey(C.CGKeyCode(code), modifierFlags(modifiers))
	return nil
}

func typeText(text string) error {
	if text == "" {
		return nil
	}
	chars := utf16.Encode([]rune(text))
	if len(chars) == 0 {
		return nil
	}
	C.meshclawTypeUnicode((*C.UniChar)(unsafe.Pointer(&chars[0])), C.long(len(chars)))
	return nil
}

func modifierFlags(modifiers []string) C.CGEventFlags {
	var flags C.CGEventFlags
	for _, modifier := range modifiers {
		switch strings.ToLower(strings.TrimSpace(modifier)) {
		case "command", "cmd", "meta":
			flags |= C.kCGEventFlagMaskCommand
		case "shift":
			flags |= C.kCGEventFlagMaskShift
		case "option", "alt":
			flags |= C.kCGEventFlagMaskAlternate
		case "control", "ctrl":
			flags |= C.kCGEventFlagMaskControl
		}
	}
	return flags
}

var keyCodes = map[string]uint16{
	"a": 0x00, "s": 0x01, "d": 0x02, "f": 0x03, "h": 0x04, "g": 0x05, "z": 0x06, "x": 0x07, "c": 0x08, "v": 0x09,
	"b": 0x0B, "q": 0x0C, "w": 0x0D, "e": 0x0E, "r": 0x0F, "y": 0x10, "t": 0x11,
	"1": 0x12, "2": 0x13, "3": 0x14, "4": 0x15, "6": 0x16, "5": 0x17, "=": 0x18, "9": 0x19, "7": 0x1A, "-": 0x1B, "8": 0x1C, "0": 0x1D, "]": 0x1E, "o": 0x1F,
	"u": 0x20, "[": 0x21, "i": 0x22, "p": 0x23, "l": 0x25, "j": 0x26, "'": 0x27, "k": 0x28, ";": 0x29, "\\": 0x2A, ",": 0x2B, "/": 0x2C, "n": 0x2D, "m": 0x2E, ".": 0x2F,
	"return": 0x24, "enter": 0x24, "tab": 0x30, "space": 0x31, "delete": 0x33, "escape": 0x35, "esc": 0x35,
	"left": 0x7B, "right": 0x7C, "down": 0x7D, "up": 0x7E,
}
