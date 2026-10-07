FROM node:24-alpine@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund
COPY web/ ./
RUN npm test && npm run build

FROM golang:1.26.8-alpine@sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY extension/ extension/
COPY migrations/ migrations/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tenantforge ./cmd/tenantforge

FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce
RUN apk add --no-cache ca-certificates tzdata && addgroup -g 10001 tenantforge && adduser -D -u 10001 -G tenantforge tenantforge && mkdir -p /data /app/web && chown -R tenantforge:tenantforge /data /app
WORKDIR /app
COPY --from=backend /out/tenantforge /usr/local/bin/tenantforge
COPY --from=web /src/web/dist/ /app/web/
COPY LICENSE THIRD_PARTY_NOTICES.md /app/
USER tenantforge
ENV TF_LISTEN=0.0.0.0:8090 TF_DATA_DIR=/data TF_WEB_DIR=/app/web
EXPOSE 8090
ENTRYPOINT ["tenantforge"]
CMD ["serve"]
