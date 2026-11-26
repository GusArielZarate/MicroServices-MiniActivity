FROM maven:3.9-eclipse-temurin-21 AS builder

WORKDIR /build
COPY pom.xml .
RUN mvn dependency:go-offline -B
COPY src ./src
RUN mvn clean package -DskipTests -B

FROM eclipse-temurin:21-jre

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends sqlite3 \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /app/data

COPY --from=builder /build/target/auth-api-1.0.0.jar app.jar

ENV DB_PATH=/app/data/auth.db
ENV PORT=8001

EXPOSE 8001

CMD ["java", "-jar", "app.jar"]
