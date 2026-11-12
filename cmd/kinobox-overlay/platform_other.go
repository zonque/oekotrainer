//go:build !darwin

package main

const singleThread = false

func platformSetup() bool { return true }
