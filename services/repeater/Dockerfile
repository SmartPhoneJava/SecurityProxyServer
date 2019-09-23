FROM golang:alpine
WORKDIR /proxy
COPY go.mod .
COPY go.sum .
RUN apk add --update git
RUN apk add --update bash && rm -rf /var/cache/apk/*
RUN go mod download
COPY . .