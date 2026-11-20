FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /nando-server ./cmd/nando-server

# Debian, not distroless: Assistants read the PDFs the Owner sends with
# poppler's programs (pdfinfo, pdftotext, pdftoppm), which Nando runs apart
# from itself (ADR 0008). The binary is still static, with its own time zones.
FROM debian:12-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends poppler-utils ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 65532 --no-create-home --shell /usr/sbin/nologin nonroot
COPY --from=build /nando-server /nando-server
USER nonroot
ENTRYPOINT ["/nando-server"]
