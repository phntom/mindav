############################
# STEP 1 build executable binary
############################
FROM golang:1.16-alpine AS builder
COPY . /app/src/
#ENV GOPROXY=https://mirrors.aliyun.com/goproxy/
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64
#COPY $GOPATH /go
WORKDIR /app/src/

RUN go build -o /app/src/builds/server /app/src/main.go
#RUN go build -o /app/src/builds/artisan /app/src/artisan.go

############################
# STEP 2 build a small server image
############################
FROM alpine

RUN mkdir /etc/configmap && \
  touch /etc/configmap/.env.json && \
  mkdir /mindav && \
  cd /mindav && \
  ln -s /etc/configmap/.env.json

# Copy .env.json
COPY --from=builder /app/src/.env.example.json /etc/configmap/.env.json
# Copy our static executable.
COPY --from=builder /app/src/builds/server /mindav/server
#COPY --from=builder /app/src/builds/artisan /bin/artisan
WORKDIR /mindav/
# Run the server binary.
ENTRYPOINT ["/mindav/server"]
EXPOSE 80
