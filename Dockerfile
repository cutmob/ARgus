# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o argus-server ./cmd/server

# Runtime stage
FROM alpine:3.19

LABEL org.opencontainers.image.title="ARGUS" \
      org.opencontainers.image.description="Real-time AI safety inspection platform powered by Gemini Live" \
      org.opencontainers.image.source="https://github.com/cutmob/ARgus"

RUN apk --no-cache add ca-certificates \
    && adduser -D -u 1001 -g argus argus

WORKDIR /app

COPY --from=builder /app/argus-server .
COPY --from=builder /app/modules ./modules
COPY --from=builder /app/configs ./configs

RUN mkdir -p /app/reports && chown -R argus:argus /app

USER argus

EXPOSE 8080

ENV PORT=8080
ENV ARGUS_MODULES_DIR=/app/modules
ENV ARGUS_REPORTS_DIR=/app/reports

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/api/v1/health || exit 1

ENTRYPOINT ["./argus-server"]
