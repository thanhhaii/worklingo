package session

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/worklingo/backend/internal/focus"
)

type CreateOptions struct {
	DurationMinutes *int    `json:"durationMinutes"`
	LearningGoal    *string `json:"learningGoal"`
}

func (o *CreateOptions) UnmarshalJSON(b []byte) error {
	type plain CreateOptions
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("options must be an object")
	}
	for _, k := range []string{"durationMinutes", "learningGoal"} {
		if string(fields[k]) == "null" {
			return errors.New(k + " cannot be null")
		}
	}
	return json.Unmarshal(b, (*plain)(o))
}
func ResolveOptions(o CreateOptions, configured time.Duration) (time.Duration, error) {
	d := configured
	if o.DurationMinutes != nil {
		switch *o.DurationMinutes {
		case 10, 20, 60:
			d = time.Duration(*o.DurationMinutes) * time.Minute
		default:
			return 0, errors.New("durationMinutes must be 10, 20, or 60")
		}
	}
	if o.LearningGoal != nil && !focus.Valid(*o.LearningGoal) {
		return 0, errors.New("invalid learningGoal")
	}
	return d, nil
}
