FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY frontend .
RUN npm run build

FROM golang:1.24-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY backend ./backend
COPY --from=frontend-builder /app/frontend/build ./backend/web
RUN go build -o /bin/remote-rumble ./backend

FROM alpine:3.21
RUN adduser -D app
USER app
WORKDIR /home/app
COPY --from=go-builder /bin/remote-rumble ./remote-rumble
EXPOSE 8080
CMD ["./remote-rumble"]
