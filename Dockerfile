# --- build stage: compile the Go binary ---
FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/app

# --- run stage: minimal image with just the binary + data ---
FROM alpine:3.20

WORKDIR /app

COPY --from=build /out/app ./app
COPY data ./data

ENTRYPOINT ["./app"]
