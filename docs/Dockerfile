FROM klakegg/hugo:ext-alpine

# Update package index before installing packages
RUN apk update && \
    apk add --no-cache git npm

COPY package.json package-lock.json ./

RUN npm install
