FROM node:20.18-alpine AS base
# Check https://github.com/nodejs/docker-node/tree/b4117f9333da4138b03a546ec926ef50a31506c3#nodealpine to understand why libc6-compat might be needed.
RUN apk add --no-cache libc6-compat vips
WORKDIR /opt/app

FROM base AS install
COPY . .
RUN npm install

FROM install AS develop
EXPOSE 3000
CMD ["npm", "run", "dev", "--", "--hostname", "0.0.0.0", "--port", "3000"]

FROM install AS build
RUN pwd && ls
RUN cp .env.local.example .env
RUN npm run build

FROM base AS production
COPY --from=build /opt/app/.next/standalone ./
COPY --from=build /opt/app/.next/static ./.next/static
# COPY --from=builder /opt/app/public ./public
ENV PORT=3000
ENV HOSTNAME="0.0.0.0"
EXPOSE 3000
CMD ["node", "server.js"]
