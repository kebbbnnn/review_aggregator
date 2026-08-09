package llm

type SummaryResponse struct {
	OverallSentiment  int      `json:"overall_sentiment"`
	Pros              []string `json:"pros"`
	Cons              []string `json:"cons"`
	CommonThemes      []string `json:"common_themes"`
	AudienceConsensus string   `json:"audience_consensus"`
	Recommendation    string   `json:"recommendation"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponseFormat struct {
	Type string `json:"type"`
}

type openAIChatRequest struct {
	Model          string               `json:"model"`
	Messages       []openAIChatMessage  `json:"messages"`
	Temperature    float64              `json:"temperature"`
	ResponseFormat openAIResponseFormat `json:"response_format"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}
