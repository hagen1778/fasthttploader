############################
# STEP 1 build executable binary
############################
FROM golang:alpine AS builder
RUN apk update && apk add --no-cache git
RUN adduser -D -g '' appuser
WORKDIR $GOPATH/src/fasthttploader/
COPY *.go ./
RUN go get -d -v
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-w -s -extldflags "-static"' -o /go/bin/fasthttploader

############################
# STEP 2 build a small image
############################
FROM scratch
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /go/bin/fasthttploader /go/bin/fasthttploader
USER appuser
ENTRYPOINT ["/go/bin/fasthttploader"]