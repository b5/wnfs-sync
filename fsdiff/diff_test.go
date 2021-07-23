package fsdiff

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestTree(t *testing.T) {
	fs := os.DirFS("testdata/one")
	got, err := Tree("a", "b", fs, fs)
	if err != nil {
		t.Fatal(err)
	}

	expect := &Delta{
		Type: DTChange,
		Name: ".",
		Deltas: []*Delta{
			{Type: DTAdd, Name: "four.txt"},
			{Type: DTChange, Name: "sub", Deltas: []*Delta{
				{Type: DTAdd, Name: "three.txt"},
				{Type: DTChange, Name: "one.txt"},
				{Type: DTRemove, Name: "two.txt"},
			}},
		},
	}

	if diff := cmp.Diff(expect, got); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}
