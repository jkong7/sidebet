FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /sidebet ./cmd/sidebet

FROM gcr.io/distroless/static:nonroot
COPY --from=build /sidebet /sidebet
ENV ADDR=:8080 DB_PATH=/data/sidebet.db SECURE=1 TRUST_PROXY=1
EXPOSE 8080
ENTRYPOINT ["/sidebet"]
