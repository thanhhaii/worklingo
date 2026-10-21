package workctx

import (
	"testing"

	"github.com/worklingo/backend/internal/dto"
)

func TestNormalise(t *testing.T) {
	w, err := Normalise(dto.WorkContext{Project: " P ", Feature: "f", Progress: "p", Blocker: "b", Notes: []string{" a ", "", "b", "c", "d", "e"}})
	if err != nil || w.Project != "P" || len(w.Notes) != 4 || w.Notes[0] != "a" {
		t.Errorf("got %+v err %v", w, err)
	}
	if _, err := Normalise(dto.WorkContext{Project: "P"}); err == nil {
		t.Error("incomplete context must be rejected")
	}
}

func TestFallbacksValid(t *testing.T) {
	for _, f := range fallbacks {
		if _, err := Normalise(f); err != nil || len(f.Notes) < 2 {
			t.Errorf("invalid fallback %q", f.Project)
		}
	}
}
