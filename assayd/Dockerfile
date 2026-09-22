ARG VERSION=dev
ARG REVISION=unknown
ARG CREATED=unknown

FROM --platform=$BUILDPLATFORM node:22.22.0-trixie-slim@sha256:465a8c8f0f4103861bcbcf3e512608394b7155eccb1955425f4ea3f672ddc53e AS web-build

WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml web/.npmrc ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.27.0-trixie@sha256:ae28539d2ef595b9a2930dd7f031d9592376829dc0eae7cb869559f7d5812c3a AS build

ARG TARGETOS
ARG TARGETARCH
WORKDIR /src/assayd
COPY assayd/go.mod assayd/go.sum ./
RUN go mod download
COPY assayd/ ./
COPY --from=web-build /src/assayd/internal/ui/dist ./internal/ui/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/assayd ./cmd/assayd

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

ARG VERSION
ARG REVISION
ARG CREATED
LABEL org.opencontainers.image.source="https://github.com/marioweid/assay" \
      org.opencontainers.image.revision=$REVISION \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.created=$CREATED

COPY --from=build /out/assayd /assayd
EXPOSE 8080
ENTRYPOINT ["/assayd"]
