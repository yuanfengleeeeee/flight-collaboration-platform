FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGET
RUN go build -o /out/app ./cmd/${TARGET}

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=build /out/app /app/app
COPY configs/config.example.yaml /app/config.yaml
ENTRYPOINT ["/app/app", "-config", "/app/config.yaml"]
