package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateRepositoryBase(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	if _, err := validate(root); err != nil {
		t.Fatal(err)
	}
}
