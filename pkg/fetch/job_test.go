package fetch

import (
	"strings"
	"testing"
)

const sampleURL = "https://prow.ci.openshift.org/view/gs/test-platform-results-public/pr-logs/pull/31837/pull-ci-openshift-origin-main-e2e-aws-ovn-serial-1of2/2117281483113631744"

func TestParseJobURL(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, raw string
		valid     bool
	}{
		{"sample", sampleURL, true},
		{"org repo", "https://prow.ci.openshift.org/view/gs/bucket/pr-logs/pull/openshift/origin/42/job/123", true},
		{"periodic", "https://prow.ci.openshift.org/view/gs/bucket/logs/job/123/", true},
		{"gcsweb", "https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/bucket/logs/job/123", true},
		{"http", strings.Replace(sampleURL, "https:", "http:", 1), false},
		{"host spoof", strings.Replace(sampleURL, "prow.ci.openshift.org", "prow.ci.openshift.org.evil.test", 1), false},
		{"userinfo", strings.Replace(sampleURL, "https://", "https://user@", 1), false},
		{"query", sampleURL + "?x=y", false},
		{"artifact", sampleURL + "/artifacts/example.json", false},
		{"traversal", "https://prow.ci.openshift.org/view/gs/bucket/logs/../123", false},
		{"invalid escape", "https://%zz", false},
		{"empty", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			job, err := ParseJobURL(test.raw)
			if (err == nil) != test.valid {
				t.Fatalf("job=%+v error=%v, valid=%v", job, err, test.valid)
			}
		})
	}
}
