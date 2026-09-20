package users

import (
	"reflect"
	"testing"
)

func TestEnsureUseraddArgsKeepsExistingHomeAndIDs(t *testing.T) {
	got := EnsureUseraddArgs("a01", "/home/a01", 1001, 1001, true)
	want := []string{"-M", "-d", "/home/a01", "-s", "/bin/bash", "-u", "1001", "-g", "1001", "a01"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestEnsureUseraddArgsCreatesHomeWhenMissing(t *testing.T) {
	got := EnsureUseraddArgs("web1", "/home/web1", 0, 0, false)
	want := []string{"-m", "-d", "/home/web1", "-s", "/bin/bash", "web1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestEnsureGroupaddArgs(t *testing.T) {
	got := EnsureGroupaddArgs("a01", 1001)
	want := []string{"-g", "1001", "a01"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
