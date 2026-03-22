FROM node:19-alpine3.16

# node deps
RUN apk add vim python3 make g++ openssh 
# net test / misc
RUN apk add bind-tools bat bash
RUN rm -rf /var/cache/apk/*

WORKDIR /app
COPY app/ ./
RUN npm install 

EXPOSE 5252

CMD ["sleep", "infinity"]
