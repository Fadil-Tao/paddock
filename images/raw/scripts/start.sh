#!/bin/bash
set -e

export DISPLAY=:99

# Virtual display
Xvfb :99 -screen 0 1280x1024x24 -ac &
sleep 1

# Window manager
openbox &

# VNC server (no password)
x11vnc -display :99 -forever -nopw -rfbport 5900 -quiet &

# noVNC web client
websockify --web /usr/share/novnc 6080 localhost:5900 &

# Browser-accessible terminal (--writable allows input)
ttyd --writable -p 7681 bash &

# Chromium with CDP exposed
chromium \
    --display=:99 \
    --no-sandbox \
    --disable-dev-shm-usage \
    --remote-debugging-port=9222 \
    --remote-debugging-address=0.0.0.0 \
    --disable-gpu \
    --no-first-run \
    --no-default-browser-check \
    about:blank &

echo "Sandbox ready"
echo "  VNC:      http://localhost:6080/vnc.html"
echo "  Terminal: http://localhost:7681"
echo "  CDP:      localhost:9222"

wait