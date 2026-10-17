FROM golang:1.22.12-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/gamestats ./cmd/server && CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/simulator ./cmd/simulator

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/ /app/
USER 65532:65532
ENV PORT=8081
EXPOSE 8081
ENTRYPOINT ["/app/gamestats"]
