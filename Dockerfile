# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Build Vars
ARG GOLANG_VERSION="latest"
ARG DEBIAN_VERSION="bookworm-slim"
ARG SERVER_VERSION="v1.0.0"
ARG SERVER_OE="development"

# Use the offical golang image to create a binary.
# This is based on Debian and sets the GOPATH to /go.
# https://hub.docker.com/_/golang
FROM golang:${GOLANG_VERSION} as builder

# Create and change to the app directory.
WORKDIR /app

# Retrieve application dependencies.
# This allows the container build to reuse cached dependencies.
# Expecting to copy go.mod and if present go.sum.
COPY go.* ./
RUN go mod download

# Copy local code to the container image.
COPY . ./

# Build the binary.
RUN go build -v -ldflags="-X 'main.Version=${SERVER_VERSION}' -X 'main.OperatingEnv=${SERVER_OE}'" -o server

# Use the official Debian slim image for a lean production container.
FROM debian:${DEBIAN_VERSION}
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Copy the binary to the production image from the builder stage.
COPY --from=builder /app/server /server

# Run the web service on container startup.
CMD ["/server"]

# [END dockerfile]
