package agent

type ToolCall struct {
	Name  string
	Input string
}

type LLMResponse struct {
	Content  string
	ToolCall *ToolCall
}
