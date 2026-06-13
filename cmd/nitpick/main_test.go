package main

import (
	"reflect"
	"testing"
)

// popPositional contract: the finding ID must be the first token; anything after
// is flags. A leading flag means no ID was given.
func TestPopPositional(t *testing.T) {
	id, rest := popPositional([]string{"RAR-01", "--evidence", "x"})
	if id != "RAR-01" || !reflect.DeepEqual(rest, []string{"--evidence", "x"}) {
		t.Errorf("id=%q rest=%v", id, rest)
	}
	id, rest = popPositional([]string{"RAR-02", "--repo", "r"})
	if id != "RAR-02" || !reflect.DeepEqual(rest, []string{"--repo", "r"}) {
		t.Errorf("id=%q rest=%v", id, rest)
	}
	id, _ = popPositional([]string{"--repo", "r"})
	if id != "" {
		t.Errorf("expected empty id (flag-first), got %q", id)
	}
}
