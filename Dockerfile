FROM golang:1.22 AS build
WORKDIR /go/app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /go/bin/goctopus ./src/.

FROM gcr.io/distroless/static-debian12
COPY --from=build /go/bin/goctopus /goctopus
EXPOSE 7890
ENTRYPOINT ["/goctopus"]
