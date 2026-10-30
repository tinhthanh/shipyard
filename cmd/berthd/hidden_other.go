//go:build !windows

package main

// runHidden is Windows' (hidden_windows.go).
func runHidden([]string) bool { return false }
