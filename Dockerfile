# Build Stage
FROM golang:1.22 as build

WORKDIR /go/src/practice-4
COPY . .

# Set GOPATH explicitly
ENV GOPATH=/go

# Print Go environment settings for debugging
RUN go env

# Run tests
RUN go test ./...

# Build the binaries
ENV CGO_ENABLED=0
RUN go install ./cmd/... ./cmd/client ./cmd/server

# Debug step to list the contents of the Go bin directory after installation
RUN ls -l $GOPATH/bin/

# Final Image Stage
FROM alpine:latest
WORKDIR /opt/practice-4

COPY entry.sh /opt/practice-4/
COPY --from=build /go/bin/* /opt/practice-4/

# Debug step to list the contents of /opt/practice-4 after copying the binaries
RUN ls -l /opt/practice-4

# Ensure the binaries have executable permissions
RUN chmod +x /opt/practice-4/client
RUN chmod +x /opt/practice-4/server

ENTRYPOINT ["/opt/practice-4/entry.sh"]
CMD ["client"]
