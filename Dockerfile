FROM golang:1.25-bookworm AS builder

ARG LDFLAGS
ARG ARCH
ARG OS

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/
COPY internal internal/

COPY Makefile Makefile

# Pass ARCH to make
RUN make release-build ARCH=${ARCH} OS=${OS}

FROM debian:bookworm-slim

ARG ARCH
ARG OS

RUN DEBIAN_FRONTEND=noninteractive apt update && apt install -y procps

WORKDIR /

# Copy only the relevant binary
COPY --from=builder /workspace/sbsh-${OS}-${ARCH} /bin/sbsh
RUN ln /bin/sbsh /bin/sb
RUN chmod 0755 /bin/sbsh /bin/sb

CMD ["/bin/sbsh", "terminal"]
