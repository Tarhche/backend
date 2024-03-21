FROM node:20.11-alpine AS base
WORKDIR /opt/app
COPY . .
RUN npm install

FROM base AS develop
EXPOSE 3000
CMD ["npm", "run", "dev", '--host']

FROM base as build
RUN npm run build

FROM node:20.11-alpine as production
WORKDIR /opt/app
COPY --from=build /opt/app/.output .
EXPOSE 3000
CMD ["node", "server/index.mjs"]
