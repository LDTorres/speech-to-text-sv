# Plan de mejoras: release e instalación

Este plan cubre la experiencia de una persona que descarga una release, la instala en una máquina limpia, la actualiza y la desinstala.

## Estado

Leyenda:

- `[x]` implementado y validado localmente
- `[ ]` pendiente de implementación, prueba manual o acción externa

## Completado

### Release y plataformas

- [x] Limitar las releases oficiales a `linux/amd64`.
- [x] Compilar la release Linux con el build tag `x11hotkey`.
- [x] Verificar el artefacto para rechazar un binario sin soporte `x11hotkey`.
- [x] Incluir únicamente los perfiles Linux y Steam Deck dentro del artefacto Linux.
- [x] Documentar que macOS usa el flujo de desarrollo, no el paquete de release Linux.

### Instalación y experiencia de uso

- [x] Añadir `install.sh --check` y `doctor.sh` para validar paquete, audio, clipboard, sesión gráfica y control externo.
- [x] Ejecutar el preflight antes de escribir configuración o descargar el modelo.
- [x] Mostrar fases claras para instalación, cambio de modelo, setup de desarrollo, build, publish, verify y uninstall.
- [x] Mostrar progreso de descarga del modelo en terminal interactiva y mensajes claros en salida no interactiva.
- [x] Validar y reportar con claridad un archivo `.env` inexistente.
- [x] Imprimir los bindings de Hyprland con la ruta absoluta de `sttdctl`.
- [x] Documentar instalación con y sin servicio, smoke checks y cómo detener el proceso manual.

### Actualización y desinstalación

- [x] Usar `~/.local/opt/sttd` como ubicación estable por defecto.
- [x] Preservar `.env` y modelos al actualizar.
- [x] Mantener la instalación anterior en `~/.local/opt/sttd.previous` para rollback manual.
- [x] Actualizar el unit de `systemd --user` para que apunte a la ubicación estable.
- [x] Ofrecer `--keep-models` y `--purge` durante la desinstalación.
- [x] Retirar el servicio y socket únicamente cuando pertenezcan a la instalación seleccionada.

### Integridad, modelos y documentación

- [x] Incluir `INSTALL.md`, `VERSION`, `doctor.sh` y scripts de ciclo de vida en el artefacto.
- [x] Generar y publicar un checksum SHA-256 para cada archivo de release.
- [x] Añadir `verify-release.sh` para verificar checksum, contenido y scripts del paquete.
- [x] Permitir fijar revisión y checksum opcional de modelos.
- [x] Reintentar descargas y eliminar archivos parciales si fallan.
- [x] Documentar requisitos, actualización, rollback, uninstall y troubleshooting básico.

### Publicación y CI

- [x] Exigir versión o bump explícito al publicar.
- [x] Rechazar publicación desde un worktree sucio.
- [x] Añadir workflows de CI y release para tests Go, Bash, ShellCheck, build, verify y publicación con checksum.

## Pendiente

### Validación manual en hardware y escritorios objetivo

- [ ] Linux/X11: confirmar trigger hold/release con hotkey, `pw-record`, `xclip` y `xdotool`.
- [ ] Linux/Hyprland: confirmar bindings start/stop, socket de control externo, `wl-copy` y `wtype`.
- [ ] Steam Deck/Gamescope: confirmar perfil toggle, sesión de usuario, audio y pegado.
- [ ] Verificar instalación, actualización, rollback y desinstalación contra un `systemd --user` real.

### Validación remota de release

- [ ] Ejecutar el workflow de CI en GitHub y confirmar que construye y verifica el archivo Linux completo.
- [ ] Crear una prerelease de prueba y comprobar descarga, checksum e instalación desde un directorio recién extraído.
- [ ] Resolver o repetir localmente el build Docker/Podman cuando el entorno permita crear el contenedor. La última validación no pudo iniciarse por un estado inválido de Podman, no por un fallo del proyecto.

### Automatización aún no implementada

- [ ] Añadir pruebas de integración con directorios temporales y fixtures para instalar, cambiar modelo, actualizar y desinstalar sin descargar modelos reales ni invocar `systemctl`.
- [ ] Ampliar `doctor.sh` o añadir `sttdctl doctor` para mostrar estado de versión, perfil, modelo configurado, trigger, audio, clipboard y servicio en una instalación existente.
- [ ] Añadir, si resulta necesario después de las pruebas manuales, un smoke test automatizado de los perfiles de release en un entorno gráfico controlado.

## Próximo paso recomendado

Hacer una prerelease `linux/amd64`, instalarla en las tres configuraciones objetivo y completar la matriz de validación manual. Con ese resultado se puede decidir si el diagnóstico ampliado y las pruebas de integración deben entrar antes de la primera release estable.
