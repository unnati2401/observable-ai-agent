                                   User
                                     │
                                     ▼
                           AI Agent (Go Application)
                                     │
        ┌────────────────────────────┼───────────────────────────┐
        │                            │                           │
        ▼                            ▼                           ▼
 Documentation Tool          YAML Generator Tool         Validation Tool
        │                            │                           │
        └────────────────────────────┼───────────────────────────┘
                                     │
                                     ▼
                            OpenTelemetry SDK
                                     │
                               OTLP (gRPC)
                                     │
                                     ▼
                     OpenTelemetry Collector
                                     │
                                     ▼
                                  Tempo
                                     │
                                     ▼
                                 Grafana
                                     │
                                     ▼
                              Trace Visualization