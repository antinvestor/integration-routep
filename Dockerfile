FROM golang:1.13 as builder

# Add Maintainer Info
LABEL maintainer="Bwire Peter <bwire517@gmail.com>"

WORKDIR /

ADD go.mod ./

RUN go mod download

ADD . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o sms-route_binary .

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /sms-route_binary /sms-route
WORKDIR /

# Run the service command by default when the container starts.
ENTRYPOINT ["/sms-route"]

# Document the port that the service listens on by default.
EXPOSE 7000
