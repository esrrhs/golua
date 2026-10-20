package core

import "testing"

func TestCAPIWrappers(t *testing.T) {
	if err := exerciseCAPI(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
