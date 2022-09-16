FROM golang:1.19 AS build-env
ARG DATE
ARG VERSION
ARG REVISION
WORKDIR /go/src/app
ADD . /go/src/app

RUN go get -d -v ./...

RUN go build -o /go/bin/smoke-test

FROM gcr.io/distroless/base

ARG DATE
ARG VERSION
ARG REVISION

COPY --from=build-env /go/bin/smoke-test /
CMD ["/some-test"]

LABEL org.opencontainers.image.created=$DATE
LABEL org.opencontainers.image.url="https://github.com/earthrise-media/smoke-test"
LABEL org.opencontainers.image.source="https://github.com/earthrise-media/smoke-test"
LABEL org.opencontainers.image.version=$VERSION
LABEL org.opencontainers.image.revision=$REVISION
LABEL org.opencontainers.image.vendor="Earthrise Media"
LABEL org.opencontainers.image.title="Smoke Testing tool"
LABEL org.opencontainers.image.description="This service provides a webhook that generates test loads during Flagger deployments"
LABEL org.opencontainers.image.authors="tingold"