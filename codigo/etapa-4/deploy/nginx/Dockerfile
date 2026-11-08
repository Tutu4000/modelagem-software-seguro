# syntax=docker/dockerfile:1

# The two apps are static files: Node only builds them. The dependencies are
# installed from the lockfile, without running any install script.
FROM node:24.22.1-alpine@sha256:06d7adf3e0ef61a13f1d28682726366f97a15aa1888c934d3ecec86c33918b53 AS web
WORKDIR /web
COPY web/package.json web/package-lock.json web/.npmrc ./
COPY web/packages/api/package.json packages/api/
COPY web/packages/auth/package.json packages/auth/
COPY web/packages/ui/package.json packages/ui/
COPY web/packages/viewer/package.json packages/viewer/
COPY web/apps/customer/package.json apps/customer/
COPY web/apps/panel/package.json apps/panel/
RUN npm ci --ignore-scripts
COPY web/ ./
RUN npm run build

# nginx serves the customer app on the public port and the panel app on the
# panel's, next to the APIs they talk to.
FROM nginxinc/nginx-unprivileged:1.30-alpine@sha256:f5116ed4606f3eb015dd4a8e28cb9b351476dc0ad4bc6e9f93b19cec130cdf03
COPY deploy/nginx/nginx.conf /etc/nginx/nginx.conf
COPY deploy/nginx/headers/ /etc/nginx/headers/
COPY --from=web /web/apps/customer/dist/ /usr/share/nginx/customer/
COPY --from=web /web/apps/panel/dist/ /usr/share/nginx/panel/
