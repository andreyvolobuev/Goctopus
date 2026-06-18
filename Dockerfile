FROM golang:1.22 AS build
WORKDIR /go/app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=none
RUN CGO_ENABLED=0 go build \
    -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /go/bin/goctopus ./src/.

# Distroless "nonroot" runs as an unprivileged user (uid 65532) by default.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /go/bin/goctopus /goctopus
EXPOSE 7890
# The binary self-probes /healthz; no shell is needed in the image.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/goctopus", "-healthcheck"]
ENTRYPOINT ["/goctopus"]
