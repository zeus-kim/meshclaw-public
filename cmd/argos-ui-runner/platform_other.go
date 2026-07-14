//go:build !darwin

package main

import "fmt"

func accessibilityTrusted(prompt bool) bool {
	return false
}

func clickPoint(x, y float64) error {
	return fmt.Errorf("Argos UI Runner is only implemented on macOS")
}

func pressKey(key string, modifiers []string) error {
	return fmt.Errorf("Argos UI Runner is only implemented on macOS")
}

func typeText(text string) error {
	return fmt.Errorf("Argos UI Runner is only implemented on macOS")
}
