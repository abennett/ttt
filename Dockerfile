FROM golang:alpine as builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-w -s"

FROM scratch
COPY --from=builder /build/ttt .
EXPOSE 8080/tcp
ENTRYPOINT ["./ttt", "serve"]
