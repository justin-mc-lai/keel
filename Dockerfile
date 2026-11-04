# Build the frontend and the binary in one stage, ship a small runtime image.
FROM node:22-alpine AS ui
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /src/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/keel ./cmd/keel

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/keel /app/keel
COPY --from=ui /src/frontend/dist /app/frontend/dist
ENV KEEL_STATE_DIR=/data
VOLUME ["/data"]
EXPOSE 8787
HEALTHCHECK --interval=30s --timeout=3s CMD ["/app/keel","healthcheck"]
ENTRYPOINT ["/app/keel","serve","-addr","0.0.0.0:8787","-assets","/app/frontend/dist","-log-json"]
