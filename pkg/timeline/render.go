// This file creates an HTML report with data and browser assets.

package timeline

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed web.html counts.js
var webFiles embed.FS

// Render embeds data and browser assets in a self-contained HTML page.
// JSON encoding escapes HTML metacharacters before insertion into the script.
func Render(data Dataset) ([]byte, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("encode timeline: %w", err)
	}
	page, err := webFiles.ReadFile("web.html")
	if err != nil {
		return nil, fmt.Errorf("read embedded HTML: %w", err)
	}
	counts, err := webFiles.ReadFile("counts.js")
	if err != nil {
		return nil, fmt.Errorf("read embedded aggregation: %w", err)
	}
	page = bytes.Replace(page, []byte("COUNTS_PLACEHOLDER"), counts, 1)
	return bytes.Replace(page, []byte("DATA_PLACEHOLDER"), encoded, 1), nil
}
