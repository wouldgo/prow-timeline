package timeline

import (
	"encoding/json"
	"strings"
	"testing"
)

const testJSON = `{"items":[{"source":"E2ETest","locator":{"type":"E2ETest","keys":{"e2e-test":"example"}},"message":{"annotations":{"status":"Passed"}},"from":"2026-11-02T17:11:09Z","to":"2026-11-02T17:11:13Z"}]}`

func TestParseTimeline(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body, wantError, wantReason string
	}{
		{"valid", testJSON, "", ""},
		{"json", `{`, "invalid interval JSON", ""},
		{"missing items", `{}`, "missing or empty", ""},
		{"empty items", `{"items":[]}`, "missing or empty", ""},
		{"invalid from", strings.Replace(testJSON, "2026-11-02T17:11:09Z", "bad", 1), "", "invalid from timestamp"},
		{"invalid to", strings.Replace(testJSON, "2026-11-02T17:11:13Z", "", 1), "", "invalid to timestamp"},
		{"reversed", strings.Replace(testJSON, "17:11:13Z", "17:11:08Z", 1), "", "ends before"},
		{"point", strings.Replace(testJSON, "17:11:13Z", "17:11:09Z", 1), "", ""},
		{"fraction timezone", strings.ReplaceAll(testJSON, "Z", ".123456789+02:00"), "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			run, err := ParseTimeline([]byte(test.body), "step/run")
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("error = %v, want %s", err, test.wantError)
				}
				return
			}
			if err != nil || run.TestCount != 1 || run.Name != "step/run" || len(run.Items) != 1 {
				t.Fatalf("run = %+v, error = %v", run, err)
			}
			event := run.Items[0]
			if event.Valid != (test.wantReason == "") || !strings.Contains(event.Reason, test.wantReason) {
				t.Fatalf("validation = %v %q, want reason %q", event.Valid, event.Reason, test.wantReason)
			}
			if run.Items[0].End < run.Items[0].Start || run.Items[0].Message.Annotations["status"] != "Passed" {
				t.Fatal("interval or outcome lost")
			}
		})
	}
}

func TestOriginalTimestampsDisplayAndExactComparisonKeys(t *testing.T) {
	t.Parallel()
	body := strings.ReplaceAll(testJSON, "Z", ".123456789+02:00")
	body = strings.Replace(body, `"source":"E2ETest"`, `"display":false,"source":"E2ETest"`, 1)
	run, err := ParseTimeline([]byte(body), "run")
	if err != nil {
		t.Fatal(err)
	}
	event := run.Items[0]
	if event.From != "2026-11-02T17:11:09.123456789+02:00" || string(event.To) != `"2026-11-02T17:11:13.123456789+02:00"` {
		t.Fatal("original timestamp changed")
	}
	if event.StartKey != "2026-11-02T15:11:09.123456789Z" || event.EndKey != "2026-11-02T15:11:13.123456789Z" {
		t.Fatalf("keys=%s %s", event.StartKey, event.EndKey)
	}
	if event.Display == nil || *event.Display {
		t.Fatal("display=false lost")
	}
	if event.Start != 1793632269123.4568 || event.End-event.Start != 4000 {
		t.Fatalf("numeric times=%f %f", event.Start, event.End)
	}
	for _, stamp := range []string{"0000-01-01T00:00:00+01:00", "9999-12-31T23:59:59-01:00"} {
		bad := strings.ReplaceAll(testJSON, "2026-11-02T17:11:09Z", stamp)
		bad = strings.ReplaceAll(bad, "2026-11-02T17:11:13Z", stamp)
		if run, err := ParseTimeline([]byte(bad), "run"); err != nil || run.Items[0].Valid || !strings.Contains(run.Items[0].Reason, "normalized UTC year") {
			t.Fatalf("UTC boundary run=%+v error=%v", run, err)
		}
	}
}

func TestOpenIntervalsPreserveNullAndUseObservedArtifactHorizon(t *testing.T) {
	t.Parallel()
	var document map[string][]map[string]any
	if err := json.Unmarshal([]byte(testJSON), &document); err != nil {
		t.Fatal(err)
	}
	document["items"] = append(document["items"], map[string]any{"source": "PodState", "from": "2026-11-02T17:11:08Z", "to": nil}, map[string]any{"source": "PodState", "from": "2026-11-02T17:11:14Z", "to": nil})
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	run, err := ParseTimeline(body, "sample")
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{1, 2} {
		event := run.Items[index]
		if !event.OpenEnd || string(event.To) != "null" || event.EndKey != "2026-11-02T17:11:14.000000000Z" {
			t.Fatalf("open interval=%+v", event)
		}
	}
	if run.Items[0].OpenEnd || run.Items[0].EndKey != "2026-11-02T17:11:13.000000000Z" {
		t.Fatal("closed interval modified")
	}
	if run, err := ParseTimeline([]byte(`{"items":[{"from":"2026-11-02T17:11:09Z"}]}`), "missing"); err != nil || run.Items[0].Valid || !strings.Contains(run.Items[0].Reason, "missing to timestamp") {
		t.Fatalf("missing end error=%v", err)
	}
	if run, err := ParseTimeline([]byte(`{"items":[{"from":"2026-11-02T17:11:09Z","to":42}]}`), "invalid"); err != nil || run.Items[0].Valid || !strings.Contains(run.Items[0].Reason, "invalid to timestamp") {
		t.Fatalf("invalid end error=%v", err)
	}
}

func TestOpenHorizonHandlesClosedEndsAndAllOpenArtifacts(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, body string }{
		{"closed end with offset", `{"items":[{"from":"2026-11-02T17:11:08Z","to":null},{"from":"2026-11-02T17:11:10Z","to":"2026-11-02T19:11:15.123456789+02:00"}]}`},
		{"all open reversed order", `{"items":[{"from":"2026-11-02T19:11:15.123456789+02:00","to":null},{"from":"2026-11-02T17:11:08Z","to":null}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			run, err := ParseTimeline([]byte(test.body), "horizon")
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range run.Items {
				if event.OpenEnd && (event.EndKey != "2026-11-02T17:11:15.123456789Z" || event.End != 1793639475123.4568) {
					t.Fatalf("horizon=%s numeric=%f", event.EndKey, event.End)
				}
			}
		})
	}
	run, err := ParseTimeline([]byte(`{"items":[{"from":"0001-01-01T00:00:00Z","to":null},{"from":"0000-01-01T00:00:00Z","to":null}]}`), "year boundary")
	if err != nil {
		t.Fatal(err)
	}
	if run.Items[1].EndKey != "0001-01-01T00:00:00.000000000Z" {
		t.Fatal("zero time confused with uninitialized horizon")
	}
}

func TestInvalidIntervalsPreserveDataAndDoNotAffectHorizon(t *testing.T) {
	t.Parallel()
	body := `{"items":[
		{"source":"MachineMonitor","from":"2026-11-02T18:06:45Z","to":"2026-11-02T18:02:07Z","valid":true,"reason":"supplied","start":999,"end":999,"startKey":"supplied","endKey":"supplied"},
		{"from":"2026-11-02T17:00:00Z","to":null},
		{"from":"2026-11-02T17:01:00Z","to":"2026-11-02T17:02:00Z","valid":false,"reason":"supplied"},
		{"from":"invalid","to":null}
	]}`
	run, err := ParseTimeline([]byte(body), "mixed")
	if err != nil || len(run.Items) != 4 {
		t.Fatalf("run=%+v error=%v", run, err)
	}
	bad := run.Items[0]
	if bad.Valid || bad.Reason != "ends before it starts" || bad.From != "2026-11-02T18:06:45Z" || string(bad.To) != `"2026-11-02T18:02:07Z"` || bad.Start != 0 || bad.End != 0 || bad.StartKey != "" || bad.EndKey != "" {
		t.Fatalf("invalid interval=%+v", bad)
	}
	if !run.Items[1].Valid || run.Items[1].EndKey != "2026-11-02T17:02:00.000000000Z" || !run.Items[2].Valid || run.Items[2].Reason != "" || run.Items[3].Valid || run.Items[3].EndKey != "" {
		t.Fatalf("validation or horizon=%+v", run.Items)
	}
	allInvalid, err := ParseTimeline([]byte(`{"items":[{"from":"bad","to":null}]}`), "invalid")
	if err != nil || allInvalid.Items[0].Valid || allInvalid.Items[0].EndKey != "" {
		t.Fatalf("all invalid run=%+v error=%v", allInvalid, err)
	}
}
