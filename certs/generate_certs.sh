#!/bin/bash
set -e

# Detecta o IP local principal da máquina de forma automática
LOCAL_IP=""
if command -v hostname >/dev/null 2>&1; then
    LOCAL_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
fi
if [ -z "$LOCAL_IP" ]; then
    LOCAL_IP=$(ip route get 1.1.1.1 2>/dev/null | awk '{print $7}')
fi
if [ -z "$LOCAL_IP" ]; then
    LOCAL_IP="127.0.0.1"
fi
echo "💻 IP local detectado: $LOCAL_IP"

# Detecta o hostname do computador
HOST_NAME=""
if command -v hostname >/dev/null 2>&1; then
    HOST_NAME=$(hostname 2>/dev/null)
fi
if [ -z "$HOST_NAME" ] && [ -f /etc/hostname ]; then
    HOST_NAME=$(cat /etc/hostname | tr -d '\r\n')
fi
if [ -z "$HOST_NAME" ]; then
    HOST_NAME=$(uname -n)
fi
if [ -z "$HOST_NAME" ]; then
    HOST_NAME="localhost"
fi
echo "💻 Hostname detectado: $HOST_NAME"

echo "🔑 1. Gerando a Autoridade Certificadora Root (CA)..."
openssl genrsa -out ca.key 2048
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt -subj "/CN=SentinelNodeRootCA"

echo "🔑 2. Gerando chaves e certificados para o Broker MQTT (Server)..."
openssl genrsa -out server.key 2048
SAN_ENTRIES="DNS:localhost,DNS:broker,DNS:sentinel_mosquitto,DNS:$HOST_NAME,DNS:$HOST_NAME.local,IP:127.0.0.1,IP:$LOCAL_IP"
if [ -n "$EXTRA_SAN" ]; then
    SAN_ENTRIES="$SAN_ENTRIES,$EXTRA_SAN"
fi
openssl req -new -key server.key -out server.csr -subj "/CN=localhost" \
  -addext "subjectAltName = $SAN_ENTRIES"
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 365 -sha256 -copy_extensions copy

echo "🔑 3. Gerando chaves e certificados para os Clientes (Câmeras e Backend)..."
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr -subj "/CN=sentinel-client"
openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client.crt -days 365 -sha256

chmod 644 ca.crt server.crt client.crt
chmod 644 server.key client.key

echo "✅ Todos os certificados foram gerados com sucesso na pasta certs/!"
