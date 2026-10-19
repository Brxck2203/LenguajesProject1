# Etapa 1: compila, ejecuta las pruebas y genera un jar con todas las dependencias.
FROM sbtscala/scala-sbt:eclipse-temurin-jammy-17.0.8.1_1_1.9.7_3.3.1 AS build
USER root
WORKDIR /app
# Primero solo la definición del build: Docker reutiliza la caché de dependencias
# mientras build.sbt y project/ no cambien.
COPY build.sbt ./
COPY project/build.properties project/plugins.sbt project/
RUN sbt -batch update
COPY src src
ARG SKIP_TESTS=false
RUN if [ "$SKIP_TESTS" = "true" ]; then sbt -batch assembly; else sbt -batch test assembly; fi

# Etapa 2: imagen mínima solo con Java para ejecutar el jar.
FROM eclipse-temurin:17-jre-jammy
RUN groupadd --system gamestats && useradd --system --gid gamestats --home /app gamestats \
    && mkdir -p /app /data/results && chown -R gamestats:gamestats /app /data
WORKDIR /app
COPY --from=build /app/target/scala-3.3.1/gamestats-analytics.jar /app/gamestats-analytics.jar
USER gamestats
ENV PORT=8080 \
    RESULTS_DIR=/data/results \
    HTTP_THREADS=8 \
    JAVA_TOOL_OPTIONS="-XX:MaxRAMPercentage=75 -Dfile.encoding=UTF-8 -Dstdout.encoding=UTF-8"
EXPOSE 8080
VOLUME ["/data/results"]
ENTRYPOINT ["java", "-jar", "/app/gamestats-analytics.jar"]
