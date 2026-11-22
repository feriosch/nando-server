package utils

// Refusal is a Tool call the Module turns down, saying why: the Tool shows
// the Connector's model its Message, and changes nothing.
type Refusal struct {
	Message string
}

func (r Refusal) Error() string { return r.Message }

// Invalid is a Refusal of something sent wrong.
func Invalid(message string) error {
	return Refusal{Message: message}
}
