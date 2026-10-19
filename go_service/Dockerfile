# Etapa 1: compila y prueba los dos binarios (servidor y simulador).
FROM golang:1.22.12-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
ARG SKIP_TESTS=false
RUN if [ "$SKIP_TESTS" != "true" ]; then go vet ./... && CGO_ENABLED=0 go test ./...; fi
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/gamestats ./cmd/server \
    && CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/simulator ./cmd/simulator

# Etapa 2: imagen mínima sin compilador, con usuario sin privilegios.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
    && mkdir -p /data/inbox && chown -R 65532:65532 /data
WORKDIR /app
COPY --from=build /out/ /app/
USER 65532:65532
ENV PORT=8081 \
    INBOX_DIR=/data/inbox
EXPOSE 8081
VOLUME ["/data/inbox"]
ENTRYPOINT ["/app/gamestats"]
