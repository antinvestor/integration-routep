FROM golang:1.12 as builder

RUN go get github.com/golang/dep/cmd/dep
WORKDIR /go/src/bitbucket.org/antinvestor/service-routep

ADD Gopkg.* ./
RUN dep ensure --vendor-only

# Copy the local package files to the container's workspace.
ADD . .

# Build the service command inside the container.
RUN go install .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o sms-route_binary .

FROM scratch
COPY --from=builder /go/src/bitbucket.org/antinvestor/service-routep/sms-route_binary /sms-route
WORKDIR /

# Run the service command by default when the container starts.
ENTRYPOINT ["/sms-route"]

# Document the port that the service listens on by default.
EXPOSE 7000
