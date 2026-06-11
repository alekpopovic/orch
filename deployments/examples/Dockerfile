FROM golang:1.25-alpine AS build

ARG TARGET=orch-server
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
RUN go build -o /out/${TARGET} ./cmd/${TARGET}

FROM alpine:3.20

ARG TARGET=orch-server
COPY --from=build /out/${TARGET} /usr/local/bin/orch-service
ENTRYPOINT ["/usr/local/bin/orch-service"]
