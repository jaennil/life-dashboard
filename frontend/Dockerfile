FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci \
  --fetch-retries=5 \
  --fetch-retry-factor=2 \
  --fetch-retry-mintimeout=20000 \
  --fetch-retry-maxtimeout=120000
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 3000
