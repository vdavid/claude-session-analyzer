package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

// writeJSON is how every `--json` answer leaves the CLI: indented, because the reader is as often a person or an agent
// reading it in a terminal as it is `jq`, and one document per invocation.
func writeJSON(w io.Writer, body any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(body); err != nil {
		return fmt.Errorf("Couldn't write the JSON: %w", err)
	}
	return nil
}
