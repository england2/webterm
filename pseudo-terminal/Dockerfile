# FROM node:19-alpine3.16
FROM node:bullseye

EXPOSE 7070

# node deps
RUN apk add python3 make g++ bat openssh
# tools
RUN apk add bind-tools vim 

RUN rm -rf /var/cache/apk/*
RUN npm install 

WORKDIR /app
COPY to-copy-inside/* ./

RUN npm install 

RUN rm -rf /var/cache/apk/*

CMD ["node", "/app/containerPseudoTerminal.js"]
