//go:build !windows

package main

// stopServing is Windows': elsewhere the service manager restarts berthd.
func stopServing(boxHome, string) {}
