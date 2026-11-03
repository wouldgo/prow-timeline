// This file finds and downloads supported artifacts from Google Cloud Storage.

package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var timelinePattern = regexp.MustCompile(`^e2e-timelines_everything_[0-9]{8}-[0-9]{6}\.json(?:\.gz)?$`)

// responseReader supplies decoded bytes for one HTTP endpoint.
type responseReader interface {
	get(context.Context, string) ([]byte, error)
}

// gcsStore owns GCS URLs, pagination and supported artifact discovery.
type gcsStore struct {
	reader responseReader
	base   string
}

// Download reads one artifact from the GCS download endpoint.
func (s *gcsStore) Download(ctx context.Context, job JobReference, name string) ([]byte, error) {
	return s.reader.get(ctx, s.base+"/download/storage/v1/b/"+url.PathEscape(job.Bucket)+"/o/"+url.PathEscape(name)+"?alt=media")
}

// Discover lists supported artifact names across all GCS pages.
func (s *gcsStore) Discover(ctx context.Context, job JobReference) ([]string, error) {
	var names []string
	seenTokens := map[string]bool{}
	seenNames := map[string]bool{}
	token := ""
	for page := 0; page < 10000; page++ {
		query := url.Values{"prefix": {job.Prefix}, "maxResults": {"1000"}, "fields": {"items(name),nextPageToken"}}
		if token != "" {
			query.Set("pageToken", token)
		}
		body, err := s.reader.get(ctx, s.base+"/storage/v1/b/"+url.PathEscape(job.Bucket)+"/o?"+query.Encode())
		if err != nil {
			return nil, fmt.Errorf("list artifacts: %w", err)
		}
		var listing struct {
			Items []struct{ Name string } `json:"items"`
			Next  string                  `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &listing); err != nil {
			return nil, fmt.Errorf("invalid GCS listing: %w", err)
		}
		for _, item := range listing.Items {
			if strings.HasPrefix(item.Name, job.Prefix) && timelinePattern.MatchString(path.Base(item.Name)) && !seenNames[item.Name] {
				seenNames[item.Name] = true
				names = append(names, item.Name)
			}
		}
		if listing.Next == "" {
			if len(names) == 0 {
				return nil, errors.New("no supported e2e-timelines_everything_YYYYMMDD-HHMMSS.json artifacts found")
			}
			return names, nil
		}
		if seenTokens[listing.Next] {
			return nil, errors.New("invalid GCS listing: repeated page token")
		}
		seenTokens[listing.Next] = true
		token = listing.Next
	}
	return nil, errors.New("GCS listing exceeds 10000 pages")
}
