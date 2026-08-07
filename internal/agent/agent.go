package agent

type Agent struct {
	llm   LLM
	tools map[string]Tool
}

func New(llm LLM, tools ...Tool) *Agent {
	toolMap := make(map[string]Tool)

	for _, t := range tools {
		toolMap[t.Definition().Name] = t
	}

	return &Agent{
		llm:   llm,
		tools: toolMap,
	}
}
