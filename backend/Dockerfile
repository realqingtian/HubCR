FROM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hubcr-api ./cmd/api \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hubcr-worker ./cmd/worker \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hubcr-migrate ./cmd/migrate

FROM docker.io/aquasec/trivy:0.72.0@sha256:cffe3f5161a47a6823fbd23d985795b3ed72a4c806da4c4df16266c02accdd6f AS trivy

FROM ghcr.io/sigstore/cosign/cosign:v3.0.6@sha256:de9c65609e6bde17e6b48de485ee788407c9502fa08b8f4459f595b21f56cd00 AS cosign

FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce

RUN apk add --no-cache ca-certificates \
    && addgroup -S hubcr \
    && adduser -S -G hubcr hubcr \
    && mkdir -p /var/lib/hubcr/trivy /var/lib/hubcr/cosign \
    && chown -R hubcr:hubcr /var/lib/hubcr

COPY --from=build /out/hubcr-api /usr/local/bin/hubcr-api
COPY --from=build /out/hubcr-worker /usr/local/bin/hubcr-worker
COPY --from=build /out/hubcr-migrate /usr/local/bin/hubcr-migrate
COPY --from=trivy /usr/local/bin/trivy /usr/local/bin/trivy
COPY --from=cosign /ko-app/cosign /usr/local/bin/cosign

USER hubcr

CMD ["/usr/local/bin/hubcr-api"]
