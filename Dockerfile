FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/mindav .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/mindav /mindav
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/mindav"]
