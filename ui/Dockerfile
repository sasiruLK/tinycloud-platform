# Multi-stage build for non-root nginx
FROM --platform=linux/arm64 nginx:alpine AS base

# Create necessary directories with nginx user ownership
RUN mkdir -p /var/cache/nginx/client_temp \
    /var/cache/nginx/proxy_temp \
    /var/cache/nginx/fastcgi_temp \
    /var/cache/nginx/uwsgi_temp \
    /var/cache/nginx/scgi_temp \
    /var/run \
    && chown -R nginx:nginx /var/cache/nginx \
    && chown -R nginx:nginx /var/run \
    && touch /var/run/nginx.pid \
    && chown nginx:nginx /var/run/nginx.pid

# Switch to nginx user (numeric UID required by Kubernetes PodSecurity)
USER 101

# Copy built static files
COPY --chown=nginx:nginx dist /usr/share/nginx/html

# Copy nginx config for SPA routing (must use port 8080 for non-root)
COPY --chown=nginx:nginx nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 8080

CMD ["nginx", "-g", "daemon off;"]
