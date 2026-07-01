/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


import org.jetbrains.kotlin.gradle.tasks.KotlinCompile
import com.google.protobuf.gradle.*
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    kotlin("jvm") version "2.4.0"
    application
    id("java")
    id("idea")
    id("com.google.protobuf") version "0.10.0"
    id("com.gradleup.shadow") version "9.4.2"
}

group = "io.opentelemetry"
version = "1.0"


val grpcVersion = "1.81.0"
val protobufVersion = "4.35.0"
val opentelemetryVersion = "1.64.0-SNAPSHOT"
val apeiroraAuditLogVersion = "0.0.1-SNAPSHOT"


repositories {
    maven {
        name = "githubPackages"
        url = uri("https://maven.pkg.github.com/apeirora/opentelemetry-java")
        credentials {
            username = System.getenv("GITHUB_ACTOR") ?: (findProperty("gpr.user") as String? ?: "")
            password = System.getenv("GITHUB_TOKEN") ?: (findProperty("gpr.key") as String? ?: "")
        }
    }
    maven {
        name = "githubPackagesAuditAPI"
        url = uri("https://maven.pkg.github.com/apeirora/AuditAPI")
        credentials {
            username = System.getenv("GITHUB_ACTOR") ?: (findProperty("gpr.user") as String? ?: "")
            password = System.getenv("GITHUB_TOKEN") ?: (findProperty("gpr.key") as String? ?: "")
        }
    }
    mavenCentral()
    gradlePluginPortal()
}



dependencies {
    implementation(platform("eu.apeirora.opentelemetry:opentelemetry-bom:${opentelemetryVersion}"))
    implementation("eu.apeirora:audit.log:${apeiroraAuditLogVersion}")
    implementation("com.google.protobuf:protobuf-java:${protobufVersion}")
    testImplementation(kotlin("test"))
    implementation(kotlin("script-runtime"))
    implementation("org.apache.kafka:kafka-clients:4.3.0")
    implementation("com.google.api.grpc:proto-google-common-protos:2.72.0")
    implementation("io.grpc:grpc-protobuf:${grpcVersion}")
    implementation("io.grpc:grpc-stub:${grpcVersion}")
    implementation("io.grpc:grpc-netty:${grpcVersion}")
    implementation("io.grpc:grpc-services:${grpcVersion}")
    implementation("eu.apeirora.opentelemetry:opentelemetry-api")
    implementation("eu.apeirora.opentelemetry:opentelemetry-sdk")
    implementation("io.opentelemetry:opentelemetry-extension-annotations:1.18.0")
    implementation("org.apache.logging.log4j:log4j-core:2.26.0")
    implementation("org.slf4j:slf4j-api:2.0.18")
    implementation("com.google.protobuf:protobuf-kotlin:${protobufVersion}")
    implementation("dev.openfeature:sdk:1.20.2")
    implementation("dev.openfeature.contrib.providers:flagd:0.14.0")

    if (JavaVersion.current().isJava9Compatible) {
        // Workaround for @javax.annotation.Generated
        // see: https://github.com/grpc/grpc-java/issues/3633
        implementation("javax.annotation:javax.annotation-api:1.3.2")
    }
}

tasks {
    shadowJar {
        duplicatesStrategy = DuplicatesStrategy.INCLUDE
        mergeServiceFiles()
    }
}

tasks.test {
    useJUnitPlatform()
}

kotlin {
  compilerOptions {
    jvmTarget.set(JvmTarget.JVM_17)
  }
}

protobuf {
    protoc {
        artifact = "com.google.protobuf:protoc:${protobufVersion}"
    }
    plugins {

        id("grpc") {
            artifact = "io.grpc:protoc-gen-grpc-java:${grpcVersion}"
        }
    }
    generateProtoTasks {
        ofSourceSet("main").forEach {
            it.plugins {
                // Apply the "grpc" plugin whose spec is defined above, without
                // options. Note the braces cannot be omitted, otherwise the
                // plugin will not be added. This is because of the implicit way
                // NamedDomainObjectContainer binds the methods.
                id("grpc") { }
            }
        }
    }
}

application {
    mainClass.set("frauddetection.MainKt")
}

tasks.jar {
    manifest.attributes["Main-Class"] = "frauddetection.MainKt"
}
