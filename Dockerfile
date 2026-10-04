FROM sbtscala/scala-sbt:eclipse-temurin-jammy-17.0.8.1_1_1.9.7_3.3.1 AS builder
WORKDIR /app
COPY build.sbt .
COPY project project
COPY src src
RUN sbt compile
EXPOSE 8080
CMD ["sbt", "run"]
