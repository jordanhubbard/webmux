//go:build !darwin

package main

func readDarwinLoad() (string, error) { return "", nil }
