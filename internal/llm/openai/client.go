package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/unnati2401/observable-ai-agent/internal/agent"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const DefaultModel = "gpt-4o-mini"

var _ agent.LLM = (*Client)(nil)

type Client struct {
	api   openai.Client
	model string
}

type Option func(*options)

type options struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

func WithAPIKey(key string) Option {
	return func(o *options) { o.apiKey = key }
}

func WithBaseURL(url string) Option {
	return func(o *options) { o.baseURL = url }
}

func WithModel(model string) Option {
	return func(o *options) { o.model = model }
}

func WithHTTPClient(client *http.Client) Option {
	return func(o *options) { o.httpClient = client }
}

func NewClient(opts ...Option) *Client {
	cfg := options{model: DefaultModel}
	for _, opt := range opts {
		opt(&cfg)
	}

	sdkOpts := make([]option.RequestOption, 0, 3)
	if cfg.apiKey != "" {
		sdkOpts = append(sdkOpts, option.WithAPIKey(cfg.apiKey))
	}
	if cfg.baseURL != "" {
		sdkOpts = append(sdkOpts, option.WithBaseURL(cfg.baseURL))
	}
	if cfg.httpClient != nil {
		sdkOpts = append(sdkOpts, option.WithHTTPClient(cfg.httpClient))
	}

	return &Client{
		api:   openai.NewClient(sdkOpts...),
		model: cfg.model,
	}
}

func (c *Client) Generate(ctx context.Context, messages []agent.Message, tools []agent.Tool) (*agent.LLMResponse, error) {
	if len(messages) == 0 {
		return nil, errors.New("openai: no messages provided")
	}

	params, err := toChatParams(c.model, messages)
	if err != nil {
		return nil, err
	}

	completion, err := c.api.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai: chat completion: %w", err)
	}

	return toLLMResponse(completion)
}
