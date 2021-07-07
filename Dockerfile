FROM golang:1.16-alpine as better-cfn-signal
WORKDIR /go/src/github.com/bdwyertech/better-cfn-signal
COPY . .
ARG VCS_REF
RUN CGO_ENABLED=0 GOFLAGS='-mod=vendor' go build -ldflags="-s -w -X main.GitCommit=$VCS_REF -X main.ReleaseVer=docker -X main.ReleaseDate=$BUILD_DATE" .

FROM library/alpine:3.14
COPY --from=better-cfn-signal /go/src/github.com/bdwyertech/better-cfn-signal/better-cfn-signal /usr/local/bin/

ARG BUILD_DATE
ARG VCS_REF

LABEL org.opencontainers.image.title="bdwyertech/better-cfn-signal" \
      org.opencontainers.image.version=$VCS_REF \
      org.opencontainers.image.description="For simplified use of CloudFormation SignalResource" \
      org.opencontainers.image.authors="Brian Dwyer <bdwyertech@github.com>" \
      org.opencontainers.image.url="https://hub.docker.com/r/bdwyertech/better-cfn-signal" \
      org.opencontainers.image.source="https://github.com/bdwyertech/better-cfn-signal.git" \
      org.opencontainers.image.revision=$VCS_REF \
      org.opencontainers.image.created=$BUILD_DATE \
      org.label-schema.name="bdwyertech/better-cfn-signal" \
      org.label-schema.description="For simplified use of CloudFormation SignalResource" \
      org.label-schema.url="https://hub.docker.com/r/bdwyertech/better-cfn-signal" \
      org.label-schema.vcs-url="https://github.com/bdwyertech/better-cfn-signal.git" \
      org.label-schema.vcs-ref=$VCS_REF \
      org.label-schema.build-date=$BUILD_DATE

RUN apk update && apk upgrade \
    && apk add --no-cache bash ca-certificates curl \
    && adduser better-cfn-signal -S -h /home/better-cfn-signal

USER better-cfn-signal
WORKDIR /home/better-cfn-signal
CMD ["bash"]
