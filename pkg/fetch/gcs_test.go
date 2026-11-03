package fetch

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type readerFunc func(context.Context, string) ([]byte, error)

func (f readerFunc) get(ctx context.Context, endpoint string) ([]byte, error) {
	return f(ctx, endpoint)
}

func TestGCSDiscovery(t *testing.T) {
	t.Parallel()
	job := JobReference{Bucket: "bucket", Prefix: "logs/job/123/artifacts/"}
	first := job.Prefix + "step one/e2e-timelines_everything_20261102-170355.json"
	second := job.Prefix + "step+two/e2e-timelines_everything_20261102-170355.json.gz"
	calls := 0
	store := &gcsStore{base: "https://storage.example", reader: readerFunc(func(_ context.Context, endpoint string) ([]byte, error) {
		u, err := url.Parse(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		query := u.Query()
		if u.Host != "storage.example" || u.Path != "/storage/v1/b/bucket/o" ||
			query.Get("prefix") != job.Prefix || query.Get("maxResults") != "1000" ||
			query.Get("fields") != "items(name),nextPageToken" {
			t.Fatalf("unexpected listing URL: %s", endpoint)
		}
		calls++
		switch calls {
		case 1:
			if query.Get("pageToken") != "" {
				t.Fatal("unexpected initial page token")
			}
			return fmt.Appendf(nil, `{"items":[{"name":%q},{"name":%q},{"name":%q}],"nextPageToken":"next/+ token"}`,
				first, strings.Replace(first, "everything_", "everything-else_", 1), first+".html"), nil
		case 2:
			if query.Get("pageToken") != "next/+ token" {
				t.Fatal("page token lost")
			}
			return fmt.Appendf(nil, `{"items":[{"name":%q},{"name":%q},{"name":"outside/e2e-timelines_everything_20261102-170355.json"}]}`, second, first), nil
		default:
			t.Fatal("unexpected extra listing request")
			return nil, nil
		}
	})}
	names, err := store.Discover(context.Background(), job)
	if err != nil || !reflect.DeepEqual(names, []string{first, second}) || calls != 2 {
		t.Fatalf("names = %v, requests = %d, error = %v", names, calls, err)
	}
}

func TestGCSDiscoveryErrors(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("reader failure")
	for _, tc := range []struct {
		name, body, want string
		err              error
	}{
		{name: "empty", body: "{}", want: "no supported"},
		{name: "malformed JSON", body: "{", want: "invalid GCS listing"},
		{name: "repeated token", body: `{"nextPageToken":"same"}`, want: "repeated page token"},
		{name: "read failure", err: sentinel, want: "list artifacts: reader failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &gcsStore{reader: readerFunc(func(context.Context, string) ([]byte, error) {
				return []byte(tc.body), tc.err
			})}
			_, err := store.Discover(context.Background(), JobReference{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("reader error was not preserved: %v", err)
			}
		})
	}
}

func TestGCSDiscoveryPageLimit(t *testing.T) {
	t.Parallel()
	calls := 0
	store := &gcsStore{reader: readerFunc(func(context.Context, string) ([]byte, error) {
		calls++
		return fmt.Appendf(nil, `{"nextPageToken":"%d"}`, calls), nil
	})}
	_, err := store.Discover(context.Background(), JobReference{})
	if err == nil || !strings.Contains(err.Error(), "exceeds 10000 pages") || calls != 10000 {
		t.Fatalf("requests = %d, error = %v", calls, err)
	}
}

func TestGCSDownloadURLAndContext(t *testing.T) {
	t.Parallel()
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request")
	job := JobReference{Bucket: "bucket", Prefix: "logs/job/123/artifacts/"}
	name := job.Prefix + "step /+?#/timeline.json"
	sentinel := errors.New("download failed")
	store := &gcsStore{base: "https://storage.example", reader: readerFunc(func(got context.Context, endpoint string) ([]byte, error) {
		if got != ctx {
			t.Fatal("context was replaced")
		}
		want := "https://storage.example/download/storage/v1/b/bucket/o/" + url.PathEscape(name) + "?alt=media"
		if endpoint != want {
			t.Fatalf("URL = %q, want %q", endpoint, want)
		}
		return nil, sentinel
	})}
	if _, err := store.Download(ctx, job, name); !errors.Is(err, sentinel) {
		t.Fatalf("error = %v", err)
	}
}
