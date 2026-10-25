package goeval

import "github.com/mahabbatzakariyayev/GoEval/providers/openai"

// OpenAIConfig configures chat/embedding models and compatible HTTP endpoints.
type OpenAIConfig = openai.Config
type OpenAIClient = openai.Client

// NewOpenAI constructs a provider without making network calls. Model selection
// and credentials are explicit; BaseURL supports compatible local endpoints.
func NewOpenAI(cfg OpenAIConfig) (*OpenAIClient, error) { return openai.New(cfg) }
