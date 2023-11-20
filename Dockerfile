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
RUN if [ -n "$MACOS_NOTARY_SECRET" ]; then make sign-macos; fi

# Production stage with only the necesssary files
FROM public.ecr.aws/debian/debian:bookworm-slim as production

# Build time configuration
ARG MONGO_URL
ARG GOOS
ARG GOARCH

ENV MONGO_URL=${MONGO_URL}
ENV EVGHOME=/static

# Put static assets where Evergreen expects them
COPY --from=build /build/clients/ ${EVGHOME}/clients/
COPY --from=build /build/public/ ${EVGHOME}/public/
COPY --from=build /build/service/templates/ ${EVGHOME}/service/templates/

RUN mkdir -p /srv && ln -s /static/clients/${GOOS}_${GOARCH}/evergreen /srv/evergreen

ENTRYPOINT ["/srv/evergreen", "service", "web"]

FROM public.ecr.aws/docker/library/golang:1.21-bookworm as build-load-initial-data
WORKDIR /build
COPY . .
RUN ["make", "bin/load-smoke-data"]

FROM mongodb/mongodb-community-server:7.0-ubuntu2204 as seeded-mongo

ARG MONGO_URL
ARG DB_NAME=mci

ENV DB_NAME=${DB_NAME}
ENV MONGO_URL=${MONGO_URL}

WORKDIR /seed
COPY --chown=mongodb:mongodb . .
COPY --chown=mongodb:mongodb --from=build-load-initial-data /build/bin/load-smoke-data ./bin/

COPY <<EOF /docker-entrypoint-initdb.d/01_load-local-data.sh
cd /seed && make load-local-data
EOF

COPY <<EOF /docker-entrypoint-initdb.d/02_replace-amboy-url.js
db = db.getSiblingDB( "${DB_NAME}" );
db.admin.updateOne({_id:"amboy"}, {"\$set": {"db_connection.url": "mongodb://mongo:27017"}})
EOF
