FROM python:3.11-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY emulator.py .

ENV PYTHONUNBUFFERED=1 \
    MQTT_BROKER_IP=broker \
    STREAM_HOST=camera-emulator \
    CERTS_DIR=/certs

EXPOSE 8087

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD python3 -c "import urllib.request; urllib.request.urlopen('http://localhost:8087/pir/presence/status')" || exit 1

ENTRYPOINT ["python3", "emulator.py"]
