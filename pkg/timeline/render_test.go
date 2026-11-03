package timeline

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestRenderSafelyEmbedsUntrustedData(t *testing.T) {
	t.Parallel()
	run := Timeline{Name: "step/run", Items: []Interval{{}}}
	attack := "</script><script>alert('x')</script>&\u2028\u2029"
	run.Items[0].Message.HumanMessage = attack
	page, err := Render(Dataset{Runs: []Timeline{run}, JobURL: attack})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(page, []byte(attack)) || bytes.Contains(page, []byte("DATA_PLACEHOLDER")) || bytes.Contains(page, []byte("innerHTML")) {
		t.Fatal("unsafe embedding or unresolved placeholder")
	}
	start := []byte(`<script id="data" type="application/json">`)
	encoded := bytes.SplitN(bytes.SplitN(page, start, 2)[1], []byte("</script>"), 2)[0]
	var decoded Dataset
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Runs[0].Items[0].Message.HumanMessage != attack {
		t.Fatal("untrusted text did not round trip")
	}
}

func TestAggregationEmbeddingPreservesUntrustedCountersAndArtifacts(t *testing.T) {
	t.Parallel()
	item := Interval{Source: "KubeEvent",
		Locator: Locator{Type: "Kind", Keys: map[string]string{"namespace": "ns", "deployment": "oauth"}},
		Message: Message{Reason: "ScalingReplicaSet", HumanMessage: "</script><script>bad()</script>",
			Annotations: map[string]string{"uid": "event-id", "count": "2"}},
		From: "2026-11-02T18:01:30Z", To: json.RawMessage(`"2026-11-02T18:01:31Z"`)}
	first := Timeline{Name: "step/a", Items: []Interval{item}}
	second := Timeline{Name: "step/b", Items: []Interval{item}}
	page, err := Render(Dataset{Runs: []Timeline{first, second}, EventCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"const Counts =", "step/a", "step/b", `"count":"2"`, `"deployment":"oauth"`, `\u003c/script\u003e`} {
		if !bytes.Contains(page, []byte(want)) {
			t.Errorf("missing embedded value %q", want)
		}
	}
	for _, forbidden := range []string{"COUNTS_PLACEHOLDER", "DATA_PLACEHOLDER", "</script><script>bad()"} {
		if bytes.Contains(page, []byte(forbidden)) {
			t.Errorf("unsafe or unresolved embedding %q", forbidden)
		}
	}
}
