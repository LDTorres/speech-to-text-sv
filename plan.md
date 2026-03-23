# Plan MVP

## Resumen

Este documento define el paso a paso para completar el MVP del daemon local-first de speech-to-text para Linux/Steam Deck. Los pasos están organizados en slices funcionales, no por paquetes aislados. Cada step debe dejar una capacidad verificable con tests automáticos y una validación manual mínima en Linux.

## Reglas del plan

- Ningún step se considera cerrado si no agrega o ajusta tests.
- Priorizar `go test ./...` y tests determinísticos con fakes.
- Cuando haya integración con OS o subprocess, cubrir contrato con tests unitarios y dejar una validación manual explícita.
- Mantener estables estas interfaces durante el plan:
  - `trigger.Watcher`
  - `audio.Recorder`
  - `transcribe.Transcriber`
  - `clipboard.Clipboard`
  - `session.Service`
- Evitar durante todo el plan:
  - introducir interfaces genéricas sin uso real
  - mover la orquestación fuera de `app` y `session`
  - agregar persistencia o HTTP

## Estructura obligatoria por step

Cada step debe escribirse y ejecutarse con esta estructura:

1. `Objetivo`
2. `Cambios`
3. `Tests automáticos`
4. `Validación manual`
5. `Done criteria`

## Step 1: Endurecer la base del daemon

### Objetivo

Dejar estable la base de ejecución del proceso largo: bootstrap, lifecycle, logging, config y loop principal.

### Cambios

- Revisar `internal/app` para asegurar start/stop limpio del watcher y manejo correcto de `ctx.Done()`.
- Agregar tests de `app` para verificar despacho de eventos `press`, `release` y `double_tap`.
- Endurecer `config` con validaciones mínimas que ya reflejen restricciones reales del MVP.
- Mantener `main.go` solo como wiring y salida controlada.

### Tests automáticos

- `TestDaemon_Run_DispatchesPressEvent`
- `TestDaemon_Run_DispatchesReleaseEvent`
- `TestDaemon_Run_DispatchesDoubleTapEvent`
- `TestDaemon_Run_StopsOnContextCancel`
- `TestConfigLoad_InvalidTimeout_ReturnsError`

### Validación manual

- Ejecutar el binario con config por defecto y confirmar que inicia y corta limpio con `SIGINT`.

### Done criteria

- El daemon arranca, queda bloqueado esperando eventos y sale limpio por contexto.
- No hay goroutines sin ownership explícito en el loop principal.

## Step 2: Hacer testeable el trigger real antes de integrarlo a Linux

### Objetivo

Reemplazar el watcher vacío por una implementación con fuente de eventos inyectable y detección explícita de `press`, `release` y `double_tap`.

### Cambios

- Definir una abstracción mínima de fuente de input en `platform` o `trigger`, sin acoplarla todavía a `evdev`.
- Implementar lógica de doble tap usando `DoubleTapWindow` de config.
- Mantener una implementación fake/in-memory para tests y una stub Linux todavía sin hardware real si hace falta.

### Tests automáticos

- `TestTriggerWatcher_EmitsPress`
- `TestTriggerWatcher_EmitsRelease`
- `TestTriggerWatcher_TwoQuickPresses_EmitDoubleTap`
- `TestTriggerWatcher_PressesOutsideWindow_DoNotEmitDoubleTap`
- `TestTriggerWatcher_Stop_UnblocksRunLoop`

### Validación manual

- Ejecutar un watcher fake desde test/integration local o comando de desarrollo y verificar la secuencia de eventos emitidos.

### Done criteria

- El módulo `trigger` produce eventos correctos de forma determinística.
- La detección de doble tap no depende de sleeps frágiles.

## Step 3: Cerrar el flujo de audio local

### Objetivo

Pasar del recorder stub a un recorder que cree un archivo temporal real y represente correctamente el ciclo start/stop.

### Cambios

- Implementar un `Recorder` con ownership claro del estado de grabación.
- Crear el archivo de salida en `TempDir` con nombre determinístico o estrategia bien definida.
- Asegurar que `Start` y `Stop` devuelvan errores explícitos en estados inválidos.
- Si todavía no se captura micrófono real, al menos escribir un archivo WAV placeholder válido o un archivo vacío controlado para habilitar el resto del flujo.

### Tests automáticos

- `TestRecorder_Start_SetsRecordingState`
- `TestRecorder_Stop_WithoutStart_ReturnsError`
- `TestRecorder_Stop_AfterStart_ReturnsRecordingWithPath`
- `TestRecorder_Start_Twice_ReturnsError`

### Validación manual

- Ejecutar una secuencia press/release simulada y confirmar que aparece un archivo bajo `STTD_AUDIO_TEMP_DIR`.

### Done criteria

- El flujo deja un artifact local consistente que el transcriber puede consumir.
- El estado `recordingActive` no se rompe ante llamadas duplicadas.

## Step 4: Completar y blindar la orquestación de sesión

### Objetivo

Cerrar la lógica de negocio central del MVP con estado en memoria explícito y tests completos de comportamiento.

### Cambios

- Endurecer `session.Service` para contemplar fallas de recorder, transcriber y clipboard sin perder consistencia.
- Exponer getters mínimos o helpers internos solo si son necesarios para testear estado.
- Asegurar que el transcript quede retenido para retry cuando falle paste, pero no cuando falle antes la transcripción.

### Tests automáticos

- Mantener los tests actuales y agregar:
- `TestSessionService_StartRecordingFailure_ReleasesState`
- `TestSessionService_TranscriptionFailure_DoesNotEnableRetry`
- `TestSessionService_CopyFailure_DoesNotLoseTranscript`
- `TestSessionService_RetryLastPaste_AfterPreviousPasteFailure_Succeeds`
- `TestSessionService_RetryLastPaste_WithoutRetryEligible_ReturnsError`

### Validación manual

- Simular errores con fakes y confirmar por logs que el flujo falla en el punto correcto.

### Done criteria

- El estado interno del session service siempre queda consistente.
- Los errores distinguen grabación, transcripción y paste.

## Step 5: Integrar `whisper.cpp` como subprocess real

### Objetivo

Hacer que la transcripción use un binario real con timeout, captura de stdout/stderr y errores accionables.

### Cambios

- Completar `transcribe.WhisperRunner` para el formato real esperado por `whisper.cpp`.
- Validar en startup, o al menos en el primer uso, que `BinaryPath` y `ModelPath` existan.
- Parsear la salida real mínima necesaria para obtener el transcript limpio.
- Mantener el runner testeable mediante comando inyectable o wrapper de ejecución.

### Tests automáticos

- `TestWhisperRunner_MissingBinaryPath_ReturnsInvalidConfig`
- `TestWhisperRunner_MissingModelPath_ReturnsInvalidConfig`
- `TestWhisperRunner_ProcessFailure_ReturnsWrappedError`
- `TestWhisperRunner_EmptyStdout_ReturnsTranscriptionError`
- `TestWhisperRunner_Success_ParsesTranscript`

### Validación manual

- Ejecutar contra un binario/modelo configurados y un archivo de audio conocido.
- Confirmar que el transcript vuelve limpio y que el timeout cancela el proceso.

### Done criteria

- El módulo `transcribe` puede ejecutar `whisper.cpp` con contexto y timeout.
- Los errores incluyen suficiente contexto para debug local.

## Step 6: Implementar clipboard y retry de paste

### Objetivo

Cerrar el tramo final del MVP: copy, paste e intento de retry usando transcript en memoria.

### Cambios

- Reemplazar el clipboard stub por una implementación Linux real o por un adapter de comandos del sistema si eso es lo más simple para MVP.
- Separar claramente copy de paste injection.
- Conservar una implementación fake para tests.
- Hacer que `RetryLastPaste` recorra exactamente el mismo flujo de copy + paste.

### Tests automáticos

- `TestClipboard_Copy_SavesTextForPaste`
- `TestClipboard_Paste_WithoutCopy_ReturnsError`
- `TestSessionService_PasteFailure_KeepsTranscriptForRetry`
- `TestSessionService_DoubleTap_RetriesLastTranscript`
- `TestSessionService_DoubleTap_AfterSuccessfulRetry_RemainsConsistent`

### Validación manual

- Con una app de texto enfocada en Linux, disparar copy/paste y luego retry por doble tap.

### Done criteria

- El transcript se pega en una app real o, si falla la inyección, queda disponible para retry.
- No se pierde el último transcript por una falla de paste.

## Step 7: Conectar trigger real + audio + transcribe + clipboard en un flujo MVP completo

### Objetivo

Tener el primer end-to-end local del producto con interacción real de usuario.

### Cambios

- Wiring final en `bootstrap` para usar implementaciones reales de trigger, audio y clipboard bajo Linux.
- Ajustar logs y notificaciones para seguir el flujo sin exponer el transcript completo.
- Añadir una prueba de integración de app con dependencias fake pero flujo completo realista.

### Tests automáticos

- `TestDaemon_Run_PressThenRelease_CompletesSessionFlow`
- `TestDaemon_Run_DoubleTap_RetriesPaste`
- `TestBootstrap_New_WiresRequiredDependencies`
- `TestEndToEndFlow_WithFakes_PressReleaseStoresAndPastesTranscript`

### Validación manual

- Mantener presionado trigger, hablar, soltar, verificar transcripción y paste.
- Hacer doble tap luego de una falla simulada de paste.

### Done criteria

- El flujo completo del MVP funciona localmente de punta a punta.
- La integración no requiere HTTP, DB ni procesos no cancelables.

## Step 8: Empaquetado operativo mínimo para uso local

### Objetivo

Dejar el MVP ejecutable y depurable como daemon local de Linux/Steam Deck.

### Cambios

- Agregar documentación operativa mínima: variables de entorno, dependencias del host, ubicación del binario y modelo.
- Definir unidad `systemd --user` de ejemplo.
- Añadir comandos de desarrollo al `Makefile` si hacen falta, sin introducir tooling innecesario.
- Si notificaciones locales quedan dentro de MVP, cerrar su implementación o dejarlas explícitamente no-op y documentadas.

### Tests automáticos

- `go test ./...` como smoke global obligatorio.
- Tests de carga de config con combinaciones reales de env.
- Si se agrega parser de unit/config local, cubrirlo con unit tests.

### Validación manual

- Levantar el servicio con `systemd --user` o correrlo foreground con `.env`.
- Reiniciar el daemon y confirmar startup/shutdown correctos.

### Done criteria

- Un desarrollador puede configurar y correr el MVP local sin leer código.
- El proyecto tiene una ruta clara de operación para Steam Deck/Linux.

## Estrategia de testing

- Cada step debe terminar con tests nuevos o endurecidos.
- Unit tests primero para estado y contratos.
- Integration-style tests con fakes para slices funcionales.
- Validación manual solo como complemento, no como sustituto del test automatizado.
- Correr `go test ./...` en cada cierre de step.
- Cuando haya concurrencia, sumar corrida con race detector en los paquetes afectados.

## Supuestos

- El plan arranca desde el skeleton actual ya compilando.
- El primer objetivo no es hardware real inmediato, sino llegar a un flujo completo verificable con reemplazos controlados.
- La implementación Linux concreta puede entrar gradualmente, siempre que cada step deje una mejora usable y comprobable.
- Este `plan.md` debe mantenerse actualizado a medida que se cierre cada step.
