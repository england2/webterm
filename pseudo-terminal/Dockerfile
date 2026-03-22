FROM node:20-bullseye

EXPOSE 7070

# node deps
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    make \
    g++ \
    openssh-client \
    fish \
    bat \
    dnsutils \
    vim \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY to-copy-inside/* ./

RUN npm install 

CMD ["node", "/app/containerPseudoTerminal.js"]
