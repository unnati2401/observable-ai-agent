# Observable AI Agent

> Observe AI agents like distributed systems using OpenTelemetry.

This is an **educational, minimal observable AI agent** written in Go. It is a
learning project for demonstrating how to trace every stage of an AI agent
execution cycle - from an HTTP request, through planning and LLM inference, to
tool execution - using OpenTelemetry, Tempo and Grafana.

> ⚠️ **Educational project - not a production framework.** This project is
> deliberately small and simplified so the observability concepts are easy to
> follow. It does not implement production-grade concerns such as prompt
> hardening, rate limiting, retry policies, multi-tool loops, authentication,
> secret management, or LLM safety guardrails. Treat it as a reference, not as
> something to deploy as-is.

## What it does

The agent performs a single round-trip tool-calling loop:

```
HTTP request
  → Agent.Run          (root span of the agent execution)
    → Planner          (decides whether to call a tool)
      → LLM.Generate   (OpenAI chat completion, sent with the registered tools)
    → Tool Lookup      (resolve the requested tool)
    → Tool Execute     (run the tool, e.g. get the current time)
    → LLM.Generate     (turn the tool result into the final answer)
  → HTTP response
```

Every stage above is a span in a single distributed trace. Errors are recorded
on the affected spans (exception events + error status) and propagate up the
tree. The whole trace is exported through the OpenTelemetry Collector to Tempo
and can be inspected in Grafana.

A built-in `get_current_time` tool lets the model call the tool, observe the
result, and answer with the actual current time - no external services needed.

## Architecture

The repository is split into two Go modules.

| Path | Module | Purpose |
| --- | --- | --- |
| `./` | `github.com/unnati2401/observable-ai-agent` | Provider-independent agent framework |
| `./app` | `github.com/unnati2401/observable-ai-agent/app` | HTTP application exposing the agent over REST |

### Agent framework

- `internal/agent` - the `Agent` that runs a conversation. It depends only on
  two interfaces, `LLM` and `Tool`, so it knows nothing about concrete
  providers. `Agent.Run` is the root of the execution trace.
- `internal/llm/openai` - an `LLM` implementation backed by the OpenAI chat
  completions API. It converts the registered tools into OpenAI function
  definitions and sends them with every request.
- `internal/tool/currenttime` - the example `Tool` that returns the current
  time for a requested timezone.
- `internal/otelutil` - a tiny helper for recording errors on spans.
- `cmd/agentdemo` - a standalone CLI that runs one conversation against a real
  OpenAI endpoint.
- `internal/integration` - end-to-end tests for the agent cycle.

### Application

- `app/cmd/server` - HTTP server entry point.
- `app/internal/api` - `HelloHandler` (`/hello`) and `ChatHandler` (`/chat`).
- `app/internal/config` - configuration loaded from environment variables.
- `app/internal/tracing` - wires the OpenTelemetry SDK to the Collector.

### Observability stack

- `infrastructure/compose/compose.yaml` - Tempo, OpenTelemetry Collector and
  Grafana, managed with Podman Compose.
- `infrastructure/otel` - Collector configuration (OTLP receiver → Tempo).
- `infrastructure/tempo` - Tempo trace storage configuration.
- `infrastructure/grafana` - auto-provisioned Tempo datasource.

## Prerequisites

- Go 1.26+
- Podman with a running machine (or Docker with a Compose v2 plugin)

## 1. Start Tempo, Grafana and the Collector

```sh
podman compose -f infrastructure/compose/compose.yaml up -d
```

| Service | Address | Purpose |
| --- | --- | --- |
| Tempo | `http://localhost:3200` | Trace storage + query API |
| OpenTelemetry Collector | `localhost:4317` (grpc) | Receives spans, forwards to Tempo |
| Grafana | `http://localhost:3000` | Trace visualization |

Stop the stack with `podman compose -f infrastructure/compose/compose.yaml down`.

## 2. Configure the LLM API key

All configuration is read from environment variables; nothing is hardcoded.

```sh
export OPENAI_API_KEY="sk-..."
# optional overrides:
export OPENAI_MODEL="gpt-4o-mini"            # default
export ADDRESS=":8080"                        # default
export OTEL_EXPORTER_OTLP_ENDPOINT="localhost:4317"  # default
```

The framework is OpenAI-compatible: point `OPENAI_API_KEY` (and optionally
`OPENAI_BASE_URL`) at any compatible provider.

## 3. Run the application

```sh
cd app
go run ./cmd/server
```

You should see:

```
Server listening on :8080 (traces -> localhost:4317)
```

## 4. Example requests

Health check:

```sh
curl http://localhost:8080/hello
# {"message":"Hello from Observable AI Agent"}
```

A question that triggers the `get_current_time` tool:

```sh
curl -X POST http://localhost:8080/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"What time is it in New York?"}'
# {"content":"The current time in New York is 10:15 AM EDT."}
```

A question that does not need a tool:

```sh
curl -X POST http://localhost:8080/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"Hello, what can you do?"}'
```

You can also run the agent as a standalone CLI (no HTTP, no tracing):

```sh
OPENAI_API_KEY="sk-..." go run ./cmd/agentdemo
```

## 5. View the trace in Grafana

1. Open <http://localhost:3000> and log in (default `admin` / `admin`).
2. Go to **Explore** (compass icon).
3. Pick the **Tempo** datasource (it is auto-provisioned).
4. Search for traces of the service:
   `{resource.service.name = "observable-ai-agent"}` (or use the Search tab and
   select the service name).
5. Click a trace and expand the span tree to inspect each stage:
   `ChatHandler` → `Agent.Run` → `Planner` → `LLM.Generate` → `Tool Lookup` →
   `Tool Execute` → `LLM.Generate`.

You can also query Tempo directly:

```sh
curl "http://localhost:3200/api/search?tags=service.name%3Dobservable-ai-agent"
```

## Example agent execution flow

Sending the request from step 4 produces a trace that looks like this in
Grafana:

```
ChatHandler                    (HTTP handler, root)
└── Agent.Run                  (agent execution; agent.name, agent.tool_names)
    ├── Planner                (decision; planner.selected_tool=get_current_time)
    │   └── LLM.Generate       (llm.provider, llm.model, llm.tools.count,
    │                            llm.tool_calls=get_current_time, llm.latency)
    ├── Tool Lookup            (tool.name=get_current_time)
    ├── Tool Execute           (tool.input, tool.arguments_valid, tool.success,
    │                            tool.execution_time)
    └── LLM.Generate           (final answer generation)
```

If any stage fails, that span and all of its ancestors are marked with an
`ERROR` status and an `exception` event, so the failure can be traced to the
exact stage (LLM call, missing tool, or tool execution).

## Running the tests

```sh
# agent framework
go test ./...

# HTTP application
cd app && go test ./...
```

## Project status

Teaching / demo project. See the individual packages and tests for details.
