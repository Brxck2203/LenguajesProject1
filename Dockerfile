# Base image con Java 17 y sbt
FROM sbtscala/scala-sbt:eclipse-temurin-jammy-17.0.8.1_1_1.9.7_3.3.1 AS builder

WORKDIR /app

# Copiar configuración y código fuente
COPY build.sbt .
COPY project project
COPY src src

# Compilar el proyecto
RUN sbt compile

# Exponer el puerto del servidor Cask
EXPOSE 8080

# Comando para iniciar el servidor
CMD ["sbt", "run"]