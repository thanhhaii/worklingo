package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/worklingo/backend/internal/practice"
)

func (h *Handlers) PracticeQueue(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.loadSession(w, r)
	if !ok {
		return
	}
	out, err := h.Practice.Queue(r.Context(), sess.ID, sess.UserID)
	if err != nil {
		h.internal(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}
func (h *Handlers) PracticeAttempts(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.loadSession(w, r)
	if !ok {
		return
	}
	// Five 4000-rune answers fit below this cap, including JSON escapes.
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	var in practice.Submission
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&in)
	if err == nil {
		var extra any
		if dec.Decode(&extra) != io.EOF {
			err = practice.ErrInvalid
		}
	}
	if err != nil {
		Error(w, http.StatusBadRequest, "bad_request", "invalid practice submission")
		return
	}
	out, err := h.Practice.Submit(r.Context(), sess.ID, sess.UserID, in)
	if errors.Is(err, practice.ErrInvalid) {
		Error(w, http.StatusBadRequest, "bad_request", "Submit 1-5 distinct session exercises with answers of 1-4000 characters and a requestId UUID.")
		return
	}
	if errors.Is(err, practice.ErrConflict) {
		Error(w, http.StatusConflict, "practice_conflict", "This request is running or its ID has already been used. Reload saved attempts before a deliberate retry.")
		return
	}
	if err != nil {
		h.internal(w, err)
		return
	}
	JSON(w, http.StatusOK, out)
}
