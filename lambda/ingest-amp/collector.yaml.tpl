receivers:
  otlp:
    protocols:
      grpc:
        endpoint: "localhost:4317"
      http:
        endpoint: "localhost:4318"

extensions:
  sigv4auth:
    service: "aps"
    region: "AMP_REGION"

exporters:
  logging:
    loglevel: debug
  prometheusremotewrite:
    endpoint: "AMP_REMOTE_WRITE_ENDPOINT"
    auth:
      authenticator: sigv4auth

service:
  extensions: [sigv4auth]
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [logging]
    metrics:
      receivers: [otlp]
      exporters: [logging, prometheusremotewrite]
