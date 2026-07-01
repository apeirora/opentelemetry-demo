# AdService Audit Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Audit-Event für jeden erfolgreichen `getAds()`-Aufruf emittieren, der festhält welcher Nutzer welche Anzeigen erhalten hat.

**Architecture:** `GlobalAuditProvider` liefert einen statischen `AuditLogger`, der nach dem Aufbau der `AdResponse` ein `AuditEvent` mit Action `READ`, Name `ad.served`, dem Nutzer aus dem Baggage und dem angefragten Ad-Target emittiert. `AuditException` wird geloggt aber nicht propagiert.

**Tech Stack:** Java 21, `audit.log` API (`eu.apeirora:audit.log:0.0.1-SNAPSHOT`), `io.opentelemetry.api.audit.GlobalAuditProvider`

---

### Task 1: `enduserId`/`sessionId` aus dem Baggage-Block herausziehen

Der aktuelle Code deklariert `enduserId` und `sessionId` als `final` innerhalb des `if (baggage != null)`-Blocks. Sie müssen außerhalb deklariert werden, damit sie im Audit-Event-Konstruktor zugänglich sind.

**Files:**
- Modify: `src/ad/src/main/java/oteldemo/AdService.java:189-202`

- [ ] **Step 1: Variablen-Deklarationen vor den Baggage-Block verschieben**

Ersetze in `getAds()` den Block:

```java
Baggage baggage = Baggage.fromContextOrNull(Context.current());
MutableContext evaluationContext = new MutableContext();
if (baggage != null) {
  final String sessionId = baggage.getEntryValue("session.id");
  span.setAttribute("session.id", sessionId);
  evaluationContext.setTargetingKey(sessionId);
  evaluationContext.add("session", sessionId);
  final String enduserId = baggage.getEntryValue("enduser.id");
  if (enduserId != null) {
    span.setAttribute("enduser.id", enduserId);
  }
} else {
  logger.info("no baggage found in context");
}
```

durch:

```java
Baggage baggage = Baggage.fromContextOrNull(Context.current());
MutableContext evaluationContext = new MutableContext();
String sessionId = null;
String enduserId = null;
if (baggage != null) {
  sessionId = baggage.getEntryValue("session.id");
  span.setAttribute("session.id", sessionId);
  evaluationContext.setTargetingKey(sessionId);
  evaluationContext.add("session", sessionId);
  enduserId = baggage.getEntryValue("enduser.id");
  if (enduserId != null) {
    span.setAttribute("enduser.id", enduserId);
  }
} else {
  logger.info("no baggage found in context");
}
```

- [ ] **Step 2: Kompilieren und prüfen**

```bash
cd src/ad && ./gradlew compileJava 2>&1 | tail -20
```

Erwartet: `BUILD SUCCESSFUL`

- [ ] **Step 3: Commit**

```bash
git add src/ad/src/main/java/oteldemo/AdService.java
git commit -m "refactor(ad): hoist sessionId/enduserId to outer scope for audit access

Assisted-by: Claude Sonnet 4.6"
```

---

### Task 2: `AuditLogger`-Feld und Imports hinzufügen

**Files:**
- Modify: `src/ad/src/main/java/oteldemo/AdService.java`

- [ ] **Step 1: Imports ergänzen**

Nach dem letzten bestehenden `import`-Statement (Zeile ~50) folgende Imports hinzufügen:

```java
import audit.log.Actor;
import audit.log.AuditEvent;
import audit.log.AuditException;
import audit.log.AuditLogger;
import audit.log.Outcome;
import audit.log.Target;
import io.opentelemetry.api.audit.GlobalAuditProvider;
```

- [ ] **Step 2: Statisches `AuditLogger`-Feld hinzufügen**

Nach dem bestehenden Feld:

```java
private static final Meter meter = GlobalOpenTelemetry.getMeter("ad");
```

folgendes Feld ergänzen:

```java
private static final AuditLogger auditLogger =
    GlobalAuditProvider.get().auditLoggerBuilder("ad").build();
```

- [ ] **Step 3: Kompilieren und prüfen**

```bash
cd src/ad && ./gradlew compileJava 2>&1 | tail -20
```

Erwartet: `BUILD SUCCESSFUL`

- [ ] **Step 4: Commit**

```bash
git add src/ad/src/main/java/oteldemo/AdService.java
git commit -m "feat(ad): add AuditLogger static field via GlobalAuditProvider

Assisted-by: Claude Sonnet 4.6"
```

---

### Task 3: Audit-Event im Erfolgsfall emittieren

**Files:**
- Modify: `src/ad/src/main/java/oteldemo/AdService.java` — Methode `getAds()` in `AdServiceImpl`

- [ ] **Step 1: Audit-Block nach `AdResponse`-Aufbau einfügen**

In `AdServiceImpl.getAds()`, direkt nach:

```java
AdResponse reply = AdResponse.newBuilder().addAllAds(allAds).build();
```

folgenden Block einfügen:

```java
String actorId = (enduserId != null) ? enduserId
               : (sessionId != null) ? sessionId
               : "unknown";
Target auditTarget = req.getContextKeysCount() > 0
    ? new Target.Resource("ad.category", req.getContextKeysList().toString())
    : new Target.Resource("ad.category", "random");
try {
  auditLogger.log(
      AuditEvent.of("READ", "ad.served")
          .actor(new Actor.User(actorId))
          .outcome(Outcome.SUCCESS)
          .target(auditTarget)
          .build());
} catch (AuditException e) {
  logger.warn("Audit delivery failed for ad.served event", e);
}
```

- [ ] **Step 2: Kompilieren**

```bash
cd src/ad && ./gradlew compileJava 2>&1 | tail -20
```

Erwartet: `BUILD SUCCESSFUL`

- [ ] **Step 3: Shadow-JAR bauen (Smoke-Check)**

```bash
cd src/ad && ./gradlew shadowJar 2>&1 | tail -10
```

Erwartet: `BUILD SUCCESSFUL`, JAR unter `build/libs/`.

- [ ] **Step 4: Commit**

```bash
git add src/ad/src/main/java/oteldemo/AdService.java
git commit -m "feat(ad): emit audit event on successful ad serving

Logs actor (enduser.id → session.id → unknown), action READ,
outcome SUCCESS and the targeted or random ad category as target.
AuditException is caught and logged as WARN — does not abort the
gRPC response.

Assisted-by: Claude Sonnet 4.6"
```
