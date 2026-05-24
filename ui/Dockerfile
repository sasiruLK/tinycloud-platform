# Single-stage build using pre-built static files
# Build the UI first with: cd ui && npm run build
# Then build this Dockerfile
FROM --platform=linux/arm64 nginx:alpine

# Copy built static files
COPY dist /usr/share/nginx/html

# Copy nginx config for SPA routing
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
