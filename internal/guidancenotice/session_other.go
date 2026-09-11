//go:build !windows && !linux && !darwin

package guidancenotice

func parentSessionID() string { return "" }
