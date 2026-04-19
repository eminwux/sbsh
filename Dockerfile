FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/
COPY internal internal/

COPY Makefile Makefile

RUN make release-build SBSH_VERSION=${VERSION} OS=${TARGETOS} ARCHS=${TARGETARCH}

FROM debian:bookworm-slim

ARG TARGETOS
ARG TARGETARCH

RUN DEBIAN_FRONTEND=noninteractive apt update && apt install -y procps

WORKDIR /

COPY --from=builder /workspace/sbsh-${TARGETOS}-${TARGETARCH} /bin/sbsh
RUN ln /bin/sbsh /bin/sb
RUN chmod 0755 /bin/sbsh /bin/sb

CMD ["/bin/sbsh", "terminal"]
