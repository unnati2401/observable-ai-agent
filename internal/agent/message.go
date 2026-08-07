package agent

type Role string

const (
	SystemRole    Role = "system"
	UserRole      Role = "user"
	AssistantRole Role = "assistant"
	ToolRole      Role = "tool"
)

// Message is a single turn in a conversation. Which fields are used depends on
// the role: AssistantRole messages may carry the ToolCall they requested, and
// ToolRole messages carry the tool result in Content plus the ToolCallID that
// links the result back to that call.
type Message struct {
	Role    Role
	Content string
	// ToolCall is set on AssistantRole messages that requested a tool.
	ToolCall *ToolCall
	// ToolCallID links a ToolRole message back to the assistant's tool call.
	ToolCallID string
}
