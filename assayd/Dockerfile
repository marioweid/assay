FROM golang:1.27.0-trixie@sha256:ae28539d2ef595b9a2930dd7f031d9592376829dc0eae7cb869559f7d5812c3a AS build

WORKDIR /src/assayd
COPY assayd/go.mod assayd/go.sum ./
RUN go mod download
COPY assayd/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/assayd ./cmd/assayd

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

COPY --from=build /out/assayd /assayd
EXPOSE 8080
ENTRYPOINT ["/assayd"]
