// This file checks job URLs and creates artifact references.

package fetch

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,220}[a-z0-9]$`)
var jobPathPattern = regexp.MustCompile(`^(?:logs/[^/]+|pr-logs/pull/(?:[^/]+/[^/]+/)?[0-9]+/[^/]+)/[0-9]+$`)

// JobReference identifies a public Prow job and its artifact prefix.
type JobReference struct{ Bucket, Prefix, URL string }

// ParseJobURL validates a supported Prow or GCS web job URL.
func ParseJobURL(raw string) (JobReference, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return JobReference{}, fmt.Errorf("unsupported Prow URL: %w", err)
	}
	allowed := map[string]string{
		"prow.ci.openshift.org":                       "/view/gs/",
		"gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com": "/gcs/",
	}
	prefix, ok := allowed[parsed.Host]
	if !ok || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !strings.HasPrefix(parsed.Path, prefix) {
		return JobReference{}, errors.New("unsupported Prow URL: use an HTTPS prow.ci.openshift.org/view/gs/ job URL without query or fragment")
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(parsed.Path, prefix), "/"), "/")
	if len(parts) < 4 || !bucketPattern.MatchString(parts[0]) {
		return JobReference{}, errors.New("unsupported Prow URL: missing bucket or job path")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "\\\x00\r\n") {
			return JobReference{}, errors.New("unsupported Prow URL: invalid path component")
		}
	}
	jobPath := strings.Join(parts[1:], "/")
	if !jobPathPattern.MatchString(jobPath) {
		return JobReference{}, errors.New("unsupported Prow URL: expected a job root ending in numeric build ID, not an artifact URL")
	}
	return JobReference{Bucket: parts[0], Prefix: jobPath + "/artifacts/", URL: raw}, nil
}
