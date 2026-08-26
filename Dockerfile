# Container image for the standalone dashboard.
#
# The plan's "later" case: this drops into the operator's Helm chart when an
# in-cluster dashboard is wanted. It is not the supported path today, and running
# it in a cluster means running an unauthenticated read view of that cluster
# behind whatever you put in front of it. Read the auth note in the README first.

# --- frontend ------------------------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /src/web

# Dependencies first, so a source-only change does not reinstall them.
COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
COPY internal/webui/dist/.gitkeep /src/internal/webui/dist/.gitkeep
RUN npm run build

# --- backend -------------------------------------------------------------------
FROM golang:1.25-alpine AS build
WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# The frontend build output, overwriting the committed placeholder.
COPY --from=web /src/internal/webui/dist/ ./internal/webui/dist/

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/dorgu-dashboard ./cmd/dorgu-dashboard

# --- runtime -------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/dorgu-dashboard /dorgu-dashboard

# Nonroot, no shell, no package manager. The image needs no capabilities: it
# opens one listening socket and reads the Kubernetes API with whatever
# credentials it is given.
USER nonroot:nonroot
EXPOSE 7171

# In a container, loopback would be unreachable from outside the pod, so the
# bind address has to be set explicitly along with the Host allowlist that keeps
# it from answering to arbitrary hostnames. Making that a deliberate argument
# rather than a default is the point.
ENTRYPOINT ["/dorgu-dashboard"]
CMD ["--host", "0.0.0.0", "--allow-host", "localhost,127.0.0.1"]
