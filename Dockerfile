# syntax=docker/dockerfile:1

FROM public.ecr.aws/docker/library/golang:1.21-bookworm as build-linux_amd64
WORKDIR /build
COPY . .
RUN ["make", "clients/linux_amd64/evergreen"]

FROM public.ecr.aws/docker/library/golang:1.21-bookworm as build-linux_s390x
WORKDIR /build
COPY . .
RUN ["make", "clients/linux_s390x/evergreen"]

FROM public.ecr.aws/docker/library/golang:1.21-bookworm as build-linux_arm64
WORKDIR /build
COPY . .
RUN ["make", "clients/linux_arm64/evergreen"]

FROM public.ecr.aws/docker/library/golang:1.21-bookworm as build-linux_ppc64le
WORKDIR /build
COPY . .
RUN ["make", "clients/linux_ppc64le/evergreen"]

FROM public.ecr.aws/docker/library/golang:1.21-bookworm as build-windows_amd64
WORKDIR /build
COPY . .
RUN ["make", "clients/windows_amd64/evergreen.exe"]

FROM public.ecr.aws/docker/library/golang:1.21-bookworm as build-darwin_amd64
WORKDIR /build
COPY . .
RUN ["make", "clients/darwin_amd64/evergreen"]

# Send the macos CLIs to the notary service for signing
ARG NOTARY_CLIENT_URL
ARG NOTARY_SERVER_URL
ARG MACOS_NOTARY_KEY
ARG MACOS_NOTARY_SECRET
ARG EVERGREEN_BUNDLE_ID

# Send the macos CLIs to the notary service for signing
RUN if [ -n "$MACOS_NOTARY_SECRET" ]; then make clients/darwin_amd64/.signed; fi

FROM public.ecr.aws/docker/library/golang:1.21-bookworm as build-darwin_arm64
WORKDIR /build
COPY . .
RUN ["make", "clients/darwin_arm64/evergreen"]

# Send the macos CLIs to the notary service for signing
ARG NOTARY_CLIENT_URL
ARG NOTARY_SERVER_URL
ARG MACOS_NOTARY_KEY
ARG MACOS_NOTARY_SECRET
ARG EVERGREEN_BUNDLE_ID

# Send the macos CLIs to the notary service for signing
RUN if [ -n "$MACOS_NOTARY_SECRET" ]; then make clients/darwin_arm64/.signed; fi

# Production stage with only the necessary files
FROM gcr.io/distroless/static as production

# Build time configuration
ARG GOOS
ARG GOARCH

ENV EVGHOME=/static

# Put static assets where Evergreen expects them
COPY --from=build-linux_amd64 /build/clients/linux_amd64/ ${EVGHOME}/clients/linux_amd64/
COPY --from=build-linux_s390x /build/clients/linux_s390x/ ${EVGHOME}/clients/linux_s390x/
COPY --from=build-linux_arm64 /build/clients/linux_arm64/ ${EVGHOME}/clients/linux_arm64/
COPY --from=build-linux_ppc64le /build/clients/linux_ppc64le/ ${EVGHOME}/clients/linux_ppc64le/
COPY --from=build-windows_amd64 /build/clients/windows_amd64/ ${EVGHOME}/clients/windows_amd64/
COPY --from=build-darwin_amd64 /build/clients/darwin_amd64/ ${EVGHOME}/clients/darwin_amd64/
COPY --from=build-darwin_arm64 /build/clients/darwin_arm64/ ${EVGHOME}/clients/darwin_arm64/

COPY ./public/ ${EVGHOME}/public/
COPY ./service/templates/ ${EVGHOME}/service/templates/

RUN ["mkdir", "-p", "/srv"]
RUN ["ln", "-s", "/static/clients/${GOOS}_${GOARCH}/evergreen", "/srv/evergreen"]

ENTRYPOINT ["/srv/evergreen", "service", "web"]
