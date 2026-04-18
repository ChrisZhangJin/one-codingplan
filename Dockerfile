# Stage 1: Build the web portal
FROM node:24-slim AS web-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm install --registry https://registry.npmmirror.com
COPY web/ ./
RUN npm run build

# Stage 2: Build the Go binary
FROM golang:1.25-alpine AS go-builder
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /app/web/dist ./internal/server/web_dist
RUN go build -trimpath -ldflags="-s -w" -o ocp ./cmd/ocp

# Stage 3: Runtime image
FROM alpine:3.23.2
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=go-builder /app/ocp /usr/local/bin/ocp
EXPOSE 8080
ENTRYPOINT ["ocp"]
CMD ["serve"]
