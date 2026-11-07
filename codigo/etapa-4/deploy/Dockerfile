# syntax=docker/dockerfile:1

# Build: binários estáticos, sem caminhos da máquina de build (C12, E2.1).
FROM golang:1.27.3-trixie@sha256:af1bfc2614ebb1dbd31af514b78740fd678850e76498762b425ecee4c5c6ea80 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

# Execução: distroless, sem shell nem gerenciador de pacotes, como não root.
# Cada estágio leva só o próprio binário.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329 AS base
USER nonroot:nonroot

FROM base AS api
COPY --from=build /out/api /api
ENTRYPOINT ["/api"]

FROM base AS garage-init
COPY --from=build /out/garage-init /garage-init
ENTRYPOINT ["/garage-init"]
