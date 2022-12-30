FROM golang:1.19.2 as build
WORKDIR /go/app
COPY . .
RUN go build -o goctopus /go/app/src/.
CMD "/bin/sh"

FROM golang:1.19.2
WORKDIR /ws
COPY --from=build ./go/app/goctopus .
EXPOSE 7890
CMD "./goctopus"
