package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/unnati2401/observable-ai-agent/internal/agent"
	"github.com/unnati2401/observable-ai-agent/internal/otelutil"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	ctx, span := tracer.Start(ctx, "LLM.Generate", trace.WithAttributes(
		attribute.String("llm.provider", "openai"),
		attribute.String("llm.model", c.model),
	))
	defer span.End()

	if len(messages) == 0 {
		err := errors.New("openai: no messages provided")
		otelutil.RecordError(span, err)
		return nil, err
	}

	params, err := toChatParams(c.model, messages, tools)
	if err != nil {
		otelutil.RecordError(span, err)
		return nil, err
	}

	span.SetAttributes(attribute.Int("llm.request_size", requestSize(params)))
	if len(tools) > 0 {
		span.SetAttributes(attribute.Int("llm.tools.count", len(tools)))
	}

	start := time.Now()
	completion, err := c.api.Chat.Completions.New(ctx, params)
	span.SetAttributes(attribute.Float64("llm.latency", time.Since(start).Seconds()))
	if err != nil {
		otelutil.RecordError(span, err)
		return nil, fmt.Errorf("openai: chat completion: %w", err)
	}

	span.SetAttributes(attribute.Int("llm.response_size", responseSize(completion)))

	response, err := toLLMResponse(completion)
	if err != nil {
		otelutil.RecordError(span, err)
		return nil, err
	}

	if response.ToolCall != nil {
		span.SetAttributes(attribute.String("llm.tool_calls", response.ToolCall.Name))
	}
	return response, nil
}
