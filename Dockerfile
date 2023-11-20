# syntax=docker/dockerfile:1

FROM public.ecr.aws/docker/library/golang:1.21-bookworm as build

# Send the macos CLIs to the notary service for signing
ARG NOTARY_CLIENT_URL
ARG NOTARY_SERVER_URL
ARG MACOS_NOTARY_KEY
ARG MACOS_NOTARY_SECRET
ARG EVERGREEN_BUNDLE_ID

WORKDIR /build
COPY . .

RUN ["make", "clis"]

# Send the macos CLIs to the notary service for signing
RUN if [ -n "$MACOS_NOTARY_SECRET" ]; then make sign-macos; fi

# Production stage with only the necesssary files
FROM gcr.io/distroless/static as production

# Build time configuration
ARG GOOS
ARG GOARCH
ARG MONGO_URL

ENV GOOS=${GOOS}
ENV GOARCH=${GOARCH}
ENV MONGO_URL=${MONGO_URL}
ENV EVGHOME=/static

# Put static assets where Evergreen expects them
COPY --from=build /build/clients/${GOOS}_${GOARCH} /
COPY --from=build /build/public/ ${EVGHOME}/public/
COPY --from=build /build/service/templates/ ${EVGHOME}/service/templates/

ENTRYPOINT ["/evergreen", "service", "web"]