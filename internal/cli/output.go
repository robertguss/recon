package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

type jsonErrorEnvelope struct {
	Error jsonErrorBody `json:"error"`
}

type jsonErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeJSONError(code string, message string, details any) error {
	return writeJSON(jsonErrorEnvelope{
		Error: jsonErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func isInteractiveTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func promptYesNo(question string, defaultYes bool) (bool, error) {
	fmt.Fprint(os.Stderr, question)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return false, err
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	if answer == "" {
		return defaultYes, nil
	}
	switch answer {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return defaultYes, nil
	}
}
