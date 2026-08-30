# Instalar y mantener sttd

Esta release está soportada en Linux `amd64`. Incluye los binarios `sttd`, `sttdctl` y el runtime de `whisper.cpp`; el modelo de transcripción se descarga durante la instalación.

## Requisitos

- Una sesión gráfica activa.
- PipeWire y `pw-record`.
- X11: `xclip` y `xdotool`.
- Wayland: `wl-copy` y `wtype`.
- `curl` para descargar el modelo.
- `systemctl --user` solo si se usa `--as-service`.

El instalador valida estos requisitos antes de cambiar archivos. Puede comprobarlos por separado con:

```bash
./install.sh --check --profile linux
```

## Instalación

Descargue el archivo `sttd-<version>-linux-amd64.tar.gz` y su archivo `.sha256`. Verifique primero la integridad:

```bash
sha256sum -c sttd-<version>-linux-amd64.tar.gz.sha256
tar -xzf sttd-<version>-linux-amd64.tar.gz
cd sttd-<version>-linux-amd64
```

Instale el perfil Linux general:

```bash
./install.sh --profile linux --language es --as-service
```

Instale para Steam Deck:

```bash
./install.sh --profile steam_deck --language es --as-service
```

Por defecto la instalación activa se guarda en `~/.local/opt/sttd`. El servicio systemd apunta a esa ubicación estable, por lo que se puede actualizar sin romper las rutas del unit.

Para instalar sin servicio:

```bash
./install.sh --profile linux --language es
~/.local/opt/sttd/sttd
```

Detenga el proceso manual con `Ctrl+C`.

## Verificación y diagnóstico

Después de instalar como servicio:

```bash
~/.local/opt/sttd/sttdctl service status
~/.local/opt/sttd/sttdctl logs tail --lines 200
~/.local/opt/sttd/doctor.sh
```

El perfil `steam_deck` y la integración `hyprland` habilitan control externo. Si está activo, puede comprobar el socket con:

```bash
~/.local/opt/sttd/sttdctl control ping
```

## Hyprland

Use la ruta absoluta que muestra el instalador. Para modo hold:

```ini
bind = $mainMod, D, exec, /home/TU_USUARIO/.local/opt/sttd/sttdctl control start
bindr = $mainMod, D, exec, /home/TU_USUARIO/.local/opt/sttd/sttdctl control stop
```

Para modo toggle:

```ini
bind = $mainMod, D, exec, /home/TU_USUARIO/.local/opt/sttd/sttdctl control toggle
```

El instalador no modifica la configuración de Hyprland automáticamente.

## Actualización y rollback

Para actualizar, descargue y extraiga una release nueva, entre en ese directorio y ejecute el mismo comando `install.sh`. El instalador conserva `.env` y los modelos descargados, reemplaza la instalación activa y guarda la versión anterior en `~/.local/opt/sttd.previous`.

Ejemplo:

```bash
cd sttd-<nueva-version>-linux-amd64
./install.sh --profile linux --language es --as-service
```

Para volver atrás, reinstale la release anterior. La carpeta `.previous` sirve como referencia y respaldo local, pero el rollback recomendado es volver a ejecutar el instalador de la versión anterior.

## Modelos e integridad

Los modelos soportados son `tiny`, `base` y `small`. Para cambiarlo:

```bash
~/.local/opt/sttd/change-model.sh --model small
```

El origen de modelos se configura en `.env` mediante `STTD_MODEL_REVISION`. El valor por defecto es `main`; para una instalación reproducible use un identificador inmutable de Hugging Face y configure también el SHA-256 correspondiente en `STTD_MODEL_SHA256_TINY`, `STTD_MODEL_SHA256_BASE` o `STTD_MODEL_SHA256_SMALL`.

## Desinstalación

Desinstalación conservando binarios y configuración:

```bash
~/.local/opt/sttd/uninstall.sh
```

Eliminar toda la instalación, modelos y logs:

```bash
~/.local/opt/sttd/uninstall.sh --purge
```
