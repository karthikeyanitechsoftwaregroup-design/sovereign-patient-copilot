package pdf

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Extract pulls plain text from a PDF byte slice.
func Extract(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("pdf.NewReader: %w", err)
	}

	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		content, err := page.GetPlainText(nil)
		if err != nil {
			// skip unreadable pages
			continue
		}
		sb.WriteString(content)
		sb.WriteRune('\n')
	}

	text := strings.TrimSpace(sb.String())
	if text == "" {
		return "", fmt.Errorf("no readable text found in PDF (scanned image?)")
	}
	return text, nil
}
