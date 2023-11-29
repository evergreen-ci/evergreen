# syntax=docker/dockerfile:1

FROM public.ecr.aws/docker/library/golang:1.21-bookworm as build
WORKDIR /build

COPY . .
RUN ["make", "clis"]

# Send the macos CLIs to the notary service for signing
ARG NOTARY_CLIENT_URL
ARG NOTARY_SERVER_URL
ARG MACOS_NOTARY_KEY
ARG MACOS_NOTARY_SECRET
ARG EVERGREEN_BUNDLE_ID

# Send the macos CLIs to the notary service for signing
RUN if [ -n "$MACOS_NOTARY_SECRET" ]; then make clients/darwin_amd64/.signed; fi

# Production stage with only the necessary files
FROM debian:bookworm-slim as production

# Build time configuration
ARG GOOS
ARG GOARCH

ENV GOOS=${GOOS}
ENV GOARCH=${GOARCH}
ENV EVGHOME=/srv

# Put static assets where Evergreen expects them
COPY --from=build /build/clients/ ${EVGHOME}/clients/
COPY ./public/ ${EVGHOME}/public/
COPY ./service/templates/ ${EVGHOME}/service/templates/

ENTRYPOINT ${EVGHOME}/clients/${GOOS}_${GOARCH}/evergreen service web
