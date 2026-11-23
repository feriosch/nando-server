package utils

// Refusal is a Tool call the Module turns down, saying why: the Tool shows
// the Connector's model its Message, and changes nothing.
type Refusal struct {
	Message string
}

func (r Refusal) Error() string { return r.Message }
