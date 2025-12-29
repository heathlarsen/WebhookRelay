# syntax=docker/dockerfile:1

FROM golang:1.23 AS build
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/webhookrelay ./cmd/webhookrelay

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/webhookrelay /app/webhookrelay

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/webhookrelay"]


