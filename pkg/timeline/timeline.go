// Package timeline models and normalizes captured timeline intervals.
package timeline

import "encoding/json"

// Interval preserves an original event and its normalized comparison times.
type Interval struct {
	Valid    bool            `json:"valid"`
	Reason   string          `json:"reason"`
	Level    string          `json:"level"`
	Source   string          `json:"source"`
	Locator  Locator         `json:"locator"`
	Message  Message         `json:"message"`
	From     string          `json:"from"`
	To       json.RawMessage `json:"to"`
	Display  *bool           `json:"display,omitempty"`
	Start    float64         `json:"start"`
	End      float64         `json:"end"`
	StartKey string          `json:"startKey"`
	EndKey   string          `json:"endKey"`
	OpenEnd  bool            `json:"openEnd,omitempty"`
}

// Timeline contains intervals from one artifact.
type Timeline struct {
	Name      string     `json:"name"`
	Items     []Interval `json:"items"`
	TestCount int        `json:"testCount"`
}

// Dataset contains all captured artifacts and their aggregate record counts.
type Dataset struct {
	JobURL     string     `json:"jobURL"`
	Runs       []Timeline `json:"runs"`
	EventCount int        `json:"eventCount"`
	TestCount  int        `json:"testCount"`
}

// Locator identifies the source resource using explicit keys.
type Locator struct {
	Type string            `json:"type"`
	Keys map[string]string `json:"keys"`
}

// Message holds the original event description and annotations.
type Message struct {
	Reason       string            `json:"reason"`
	Cause        string            `json:"cause"`
	HumanMessage string            `json:"humanMessage"`
	Annotations  map[string]string `json:"annotations"`
}

// IsTest reports whether the interval names an E2E test.
func (i Interval) IsTest() bool {
	return i.Source == "E2ETest" && i.Locator.Keys["e2e-test"] != ""
}
