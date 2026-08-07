package agent

// ToolCall is a request by the model to execute a tool.
type ToolCall struct {
	// ID identifies the tool call within the conversation and links a tool
	// result back to it. It is assigned by the LLM provider.
	ID    string
	Name  string
	Input string
}

// LLMResponse is either the model's final answer or a request to execute a
// tool.
type LLMResponse struct {
	Content  string
	ToolCall *ToolCall
}
