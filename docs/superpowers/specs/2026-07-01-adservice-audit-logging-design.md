# Design: Audit Logging im AdService

**Date:** 2026-07-01
**Scope:** `src/ad/src/main/java/oteldemo/AdService.java`
**Signal:** `eu.apeirora:audit.log` via `GlobalAuditProvider`

---

## Ziel

Auditierung der Ad-Serving-Entscheidung in `getAds()` — welcher Nutzer hat wann welche Anzeigen erhalten. Compliance-relevanter Datenzugriff (DSGVO, CCPA): personenbezogene Werbung muss nachvollziehbar sein.

Erster Schritt: nur der Erfolgsfall. Fehlerfall und Feature-Flag-Events folgen separat.

---

## Initialisierung

Statisches Feld analog zu `tracer` und `meter`:

```java
private static final AuditLogger auditLogger =
    GlobalAuditProvider.get().auditLoggerBuilder("ad").build();
```

---

## Event-Definition

| Feld | Wert |
|---|---|
| `action` | `"READ"` |
| `name` | `"ad.served"` |
| `actor` | `Actor.User(enduserId)` — Fallback auf `sessionId`, dann `"unknown"` |
| `outcome` | `Outcome.SUCCESS` |
| `target` (targeted) | `new Target.Resource("ad.category", req.getContextKeysList().toString())` |
| `target` (random) | `new Target.Resource("ad.category", "random")` |

Der Actor wird aus den Baggage-Feldern `enduser.id` / `session.id` gebaut. Da diese aktuell als `final` Variablen innerhalb des `if (baggage != null)`-Blocks deklariert sind, müssen sie vor dem Block als `String enduserId = null; String sessionId = null;` deklariert und im Block zugewiesen werden — minimale Umstrukturierung, kein Logikwechsel.

---

## Platzierung im Code

Nach `AdResponse reply = AdResponse.newBuilder().addAllAds(allAds).build()`, vor `responseObserver.onNext(reply)`:

```java
AdResponse reply = AdResponse.newBuilder().addAllAds(allAds).build();

// Audit: Ad-Serving-Entscheidung
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

responseObserver.onNext(reply);
```

---

## Fehlerbehandlung

`AuditException` wird geloggt (`WARN`), aber **nicht als gRPC-Fehler propagiert**. Der Ad-Request soll nicht scheitern, weil das Audit-Sink nicht antwortet. Das ist eine bewusste Entscheidung: Observability-Ausfall blockiert nicht den User-Pfad.

---

## Scope-Grenzen

- Nur `getAds()` Erfolgsfall — kein `StatusRuntimeException`-Pfad
- Keine Änderungen an anderen Services
- Keine neuen Klassen oder Dateien — alles bleibt in `AdService.java`
- Gesamtänderung: ~20 Zeilen additiv

---

## Offene Punkte (nicht in diesem PR)

- `AuditProvider`-Konfiguration (Exporter-Endpunkt, TLS) via `otel.audit.exporter`
- Fehlerfall-Auditierung (`Outcome.FAILURE` im catch-Block)
- Feature-Flag-Aktivierung auditieren (`adManualGc`, `adHighCpu`)
