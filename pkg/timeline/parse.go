// This file checks event timestamps and sets bounds for open intervals.

package timeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ParseTimeline decodes and validates one artifact, preserving original fields.
// Invalid intervals retain their original fields and a validation reason.
// Open intervals end at the latest valid timestamp in that artifact.
func ParseTimeline(body []byte, name string) (Timeline, error) {
	var run Timeline
	if err := json.Unmarshal(body, &run); err != nil {
		return run, fmt.Errorf("invalid interval JSON: %w", err)
	}
	run.Name = name
	run.TestCount = 0
	if len(run.Items) == 0 {
		return run, errors.New("missing or empty .items intervals")
	}
	var horizon time.Time
	haveHorizon := false
	for index := range run.Items {
		event := &run.Items[index]
		event.Valid = false
		event.Reason = ""
		event.OpenEnd = false
		event.Start, event.End = 0, 0
		event.StartKey, event.EndKey = "", ""
		to, err := event.normalize()
		if err != nil {
			event.Reason = err.Error()
		} else {
			event.Valid = true
			if !haveHorizon || to.After(horizon) {
				horizon = to
				haveHorizon = true
			}
		}
		if event.IsTest() {
			run.TestCount++
		}
	}
	for index := range run.Items {
		event := &run.Items[index]
		if event.Valid && event.OpenEnd {
			event.End = milliseconds(horizon)
			event.EndKey = horizon.UTC().Format("2006-01-02T15:04:05.000000000Z")
		}
	}
	return run, nil
}

// normalize validates one interval and computes its closed comparison bounds.
// An open interval initially ends at its own start until the artifact is known.
func (i *Interval) normalize() (time.Time, error) {
	from, err := time.Parse(time.RFC3339Nano, i.From)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid from timestamp %q: %w", i.From, err)
	}
	to := from
	i.OpenEnd = bytes.Equal(i.To, []byte("null"))
	if !i.OpenEnd {
		var originalTo string
		if len(i.To) == 0 {
			return time.Time{}, errors.New("missing to timestamp (explicit null denotes an open interval)")
		}
		if err := json.Unmarshal(i.To, &originalTo); err != nil {
			return time.Time{}, fmt.Errorf("invalid to timestamp: %w", err)
		}
		to, err = time.Parse(time.RFC3339Nano, originalTo)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid to timestamp %q: %w", originalTo, err)
		}
	}
	if to.Before(from) {
		return time.Time{}, errors.New("ends before it starts")
	}
	if from.UTC().Year() < 0 || from.UTC().Year() > 9999 || to.UTC().Year() < 0 || to.UTC().Year() > 9999 {
		return time.Time{}, errors.New("invalid timestamp: normalized UTC year must be 0000–9999")
	}
	i.Start = milliseconds(from)
	i.End = milliseconds(to)
	i.StartKey = from.UTC().Format("2006-01-02T15:04:05.000000000Z")
	i.EndKey = to.UTC().Format("2006-01-02T15:04:05.000000000Z")

	return to, nil
}

func milliseconds(t time.Time) float64 {
	return float64(t.Unix())*1000 + float64(t.Nanosecond())/1e6
}
