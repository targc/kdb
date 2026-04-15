FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o operator .

FROM alpine:3.20
COPY --from=build /app/operator /operator
ENTRYPOINT ["/operator"]
