ThisBuild / scalaVersion := "3.3.1"
ThisBuild / organization := "cr.ac.itcr"
ThisBuild / version := "1.0.0"

lazy val root = (project in file("."))
  .settings(
    name := "scala-analytics",
    libraryDependencies ++= Seq(
      "com.lihaoyi" %% "ujson" % "3.1.3",
      "org.scalameta" %% "munit" % "1.0.0" % Test
    ),
    scalacOptions ++= Seq("-encoding", "utf8", "-deprecation", "-feature", "-Wunused:all"),
    Compile / mainClass := Some("gamestats.Main"),
    // Pruebas en un proceso aparte: el servidor no queda escuchando durante `sbt test`.
    Test / fork := true,
    // Jar ejecutable con todas las dependencias: java -jar gamestats-analytics.jar
    assembly / mainClass := Some("gamestats.Main"),
    assembly / assemblyJarName := "gamestats-analytics.jar",
    assembly / assemblyMergeStrategy := {
      case path if path.endsWith("module-info.class") => MergeStrategy.discard
      case other =>
        val defaultStrategy = (assembly / assemblyMergeStrategy).value
        defaultStrategy(other)
    }
  )
