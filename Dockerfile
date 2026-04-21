FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/todo-go .

FROM alpine:3.20
RUN adduser -D -u 10001 todo \
    && mkdir -p /data \
    && chown todo:todo /data

WORKDIR /app
COPY --from=build /out/todo-go /app/todo-go

USER todo

ENV TODO_GO_DATA=/data/tasks.json
ENV TODO_GO_ADDR=0.0.0.0:8080

EXPOSE 8080

ENTRYPOINT ["/app/todo-go"]
CMD ["serve"]
