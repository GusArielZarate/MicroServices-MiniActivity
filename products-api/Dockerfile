FROM node:20-bookworm-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    make \
    g++ \
    sqlite3 \
    && rm -rf /var/lib/apt/lists/*

COPY package.json .
RUN npm install --omit=dev

COPY . .

RUN mkdir -p /app/data

ENV DB_PATH=/app/data/products.db
ENV PORT=8003

EXPOSE 8003

CMD ["node", "index.js"]
