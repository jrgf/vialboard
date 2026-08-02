FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /vialboard ./cmd/vialboard

FROM alpine:3.23

RUN apk add --no-cache ca-certificates
COPY --from=build /vialboard /usr/local/bin/vialboard

USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["vialboard", "healthcheck"]
ENTRYPOINT ["vialboard"]
