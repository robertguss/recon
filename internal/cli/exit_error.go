package cli

type ExitError struct {
	Code    int
	Message string
}

func (e ExitError) Error() string {
	return e.Message
}
