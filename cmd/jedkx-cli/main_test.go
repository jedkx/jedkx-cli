package main

import (
	"reflect"
	"testing"
)

func TestParsePicks(t *testing.T) {
	for in, want := range map[string][]int{"2": {2}, "2 4": {2, 4}, "4,2": {2, 4}, "2-4": {2, 3, 4}, "1+1": {1}} {
		if got, ok := parsePicks(in, 5); !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("%q = %v %v, want %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"0", "6", "x", "4-2", "2-"} {
		if _, ok := parsePicks(bad, 5); ok {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}
