FROM golang:1.25 AS builder
ARG VERSION=v0.0.0-dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w \
      -X k8s.io/component-base/version.gitVersion=${VERSION} \
      -X k8s.io/component-base/version.gitCommit=${COMMIT}" \
    -o /cloud-provider-opentelekomcloud ./cmd/cloud-controller-manager

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /cloud-provider-opentelekomcloud /bin/cloud-provider-opentelekomcloud
USER 65532:65532
ENTRYPOINT ["/bin/cloud-provider-opentelekomcloud"]
