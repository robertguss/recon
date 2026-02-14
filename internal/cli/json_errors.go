package cli

import "errors"

func exitJSONCommandError(err error) error {
	code, details := classifyJSONCommandError(err)
	_ = writeJSONError(code, err.Error(), details)
	return ExitError{Code: 2}
}

func classifyJSONCommandError(err error) (string, any) {
	var notInitialized dbNotInitializedError
	if errors.As(err, &notInitialized) {
		return "not_initialized", map[string]any{"path": notInitialized.Path}
	}
	return "internal_error", nil
}
