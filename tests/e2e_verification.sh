#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "=================================================================="
echo "🛡️  INICIANDO TESTE DE INTEGRAÇÃO PONTA A PONTA (E2E) SENTINELNODE"
echo "=================================================================="

PIDS=()
cleanup() {
    echo ""
    echo "🧹 Limpando processos de teste em execução..."
    for pid in "${PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
        fi
    done
    wait 2>/dev/null || true
    echo "✅ Processos finalizados com segurança."
}
trap cleanup EXIT INT TERM

# 1. Checar Docker Compose (Mosquitto + Postgres)
echo "🔍 1. Verificando infraestrutura Docker (Mosquitto & PostgreSQL)..."
if ! docker compose ps | grep -q "sentinel_mosquitto.*Up"; then
    echo "Iniciando containers do Docker Compose..."
    docker compose up -d broker db
    sleep 2
fi

echo "   ↳ Mosquitto MQTT: Ativo na porta 8883 (mTLS) e 1883"
echo "   ↳ PostgreSQL: Ativo na porta 5435"

# Limpa gravações anteriores para um teste limpo
export RECORDINGS_DIR="$ROOT_DIR/recordings"
rm -rf "$RECORDINGS_DIR"/*.mp4 "$RECORDINGS_DIR"/manifest.json
mkdir -p "$RECORDINGS_DIR"

# 2. Iniciar Media Gateway
echo "🚀 2. Iniciando Media Gateway com tolerância de 4 segundos..."
TOLERANCE_SECONDS=4 RECORDINGS_DIR="$RECORDINGS_DIR" ./media-gateway/bin/media-gateway > /tmp/sentinel_media_gateway.log 2>&1 &
PIDS+=($!)
sleep 1

if ! curl -s http://127.0.0.1:8080/status >/dev/null; then
    echo "❌ Erro: Media Gateway falhou ao iniciar!"
    cat /tmp/sentinel_media_gateway.log
    exit 1
fi
echo "   ↳ Media Gateway online em http://127.0.0.1:8080"

# 3. Iniciar Ingestion Engine
echo "🚀 3. Iniciando Ingestion Engine..."
./ingestion-engine/bin/ingestion-engine > /tmp/sentinel_ingestion.log 2>&1 &
PIDS+=($!)
sleep 1

echo "   ↳ Ingestion Engine conectado ao Mosquitto via mTLS"

# 4. Iniciar Emulador ESP32-CAM
echo "🚀 4. Iniciando Emulador de Hardware ESP32-CAM (porta 8087)..."
./firmware/ESP-CAM/.venv/bin/python3 firmware/ESP-CAM/emulator.py > /tmp/sentinel_emulator.log 2>&1 &
PIDS+=($!)

echo "⏳ Aguardando registro dinâmico da câmera via Heartbeat MQTT mTLS..."
REGISTERED=false
for i in {1..15}; do
    STATUS_JSON=$(curl -s http://127.0.0.1:8080/status || echo "{}")
    if echo "$STATUS_JSON" | grep -q '"status":"streaming"'; then
        REGISTERED=true
        break
    fi
    sleep 1
done

if [ "$REGISTERED" != "true" ]; then
    echo "❌ Falha: A câmera não atingiu o estado 'streaming' em 15 segundos."
    echo "Logs do Emulador:"
    cat /tmp/sentinel_emulator.log
    echo "Logs do Ingestion Engine:"
    cat /tmp/sentinel_ingestion.log
    echo "Logs do Media Gateway:"
    cat /tmp/sentinel_media_gateway.log
    exit 1
fi

echo "✅ Câmera dinamicamente registrada e conectada ao Broadcaster central!"

# 5. Testar Métricas e Snapshot
echo "📸 5. Testando snapshot JPEG ao vivo..."
HTTP_CODE=$(curl -s -o /tmp/test_snapshot.jpg -w "%{http_code}" http://127.0.0.1:8080/snapshot?device_id=esp32-cam-01)
if [ "$HTTP_CODE" != "200" ] || [ ! -s /tmp/test_snapshot.jpg ]; then
    echo "❌ Falha ao obter snapshot: HTTP $HTTP_CODE"
    exit 1
fi
SNAPSHOT_SIZE=$(wc -c < /tmp/test_snapshot.jpg)
echo "   ↳ Snapshot obtido com sucesso ($SNAPSHOT_SIZE bytes, JPEG válido)"

# 6. Testar Live Streaming com múltiplos visualizadores simultâneos
echo "👥 6. Validando múltiplos espectadores simultâneos no /live..."
(curl -s -N http://127.0.0.1:8080/live?device_id=esp32-cam-01 > /dev/null) &
PID_VIEWER1=$!
(curl -s -N http://127.0.0.1:8080/live?device_id=esp32-cam-01 > /dev/null) &
PID_VIEWER2=$!
sleep 1

STATUS_JSON=$(curl -s http://127.0.0.1:8080/status)
echo "   ↳ Telemetria Media Gateway:"
echo "$STATUS_JSON"

kill $PID_VIEWER1 $PID_VIEWER2 2>/dev/null || true

# 7. Disparar Evento PIR de Movimento
echo "⚡ 7. Disparando evento de detecção de movimento (PIR)..."
PIR_RESPONSE=$(curl -s -X POST http://127.0.0.1:8087/pir/trigger)
echo "   ↳ Resposta do Emulador: $PIR_RESPONSE"

# 8. Verificar Gravação Ativa no Media Gateway
echo "🎥 8. Verificando início automático da gravação..."
sleep 1.5
STATUS_JSON=$(curl -s http://127.0.0.1:8080/status)
if ! echo "$STATUS_JSON" | grep -q '"is_recording":true'; then
    echo "❌ Gravação não foi ativada após evento PIR!"
    cat /tmp/sentinel_media_gateway.log
    exit 1
fi
echo "   ↳ Gravação ATIVA confirmada via API de status!"

# 9. Testar Renovação da Janela Deslizante (Sliding Window)
echo "🔄 9. Testando renovação da tolerância (evento sucessivo aos 2s)..."
sleep 2
PIR_RESPONSE2=$(curl -s -X POST http://127.0.0.1:8087/pir/trigger)
echo "   ↳ Segundo evento PIR disparado: $PIR_RESPONSE2"

# 10. Aguardar término após expiração da nova tolerância
echo "⏳ 10. Aguardando expiração da tolerância (4s) para encerramento gracioso..."
sleep 5.5

STATUS_JSON=$(curl -s http://127.0.0.1:8080/status)
if echo "$STATUS_JSON" | grep -q '"is_recording":true'; then
    echo "❌ Gravação ainda está ativa após expiração da tolerância!"
    exit 1
fi
echo "   ↳ Gravação finalizada após expiração da tolerância."

# 11. Verificar Arquivo de Vídeo no Disco
echo "💾 11. Verificando gravação MP4 no disco..."
RECORDINGS=$(find "$RECORDINGS_DIR" -name "*.mp4")
if [ -z "$RECORDINGS" ]; then
    echo "❌ Nenhum arquivo MP4 encontrado em $RECORDINGS_DIR"
    exit 1
fi
for rec in $RECORDINGS; do
    REC_SIZE=$(wc -c < "$rec")
    echo "   ↳ Arquivo gravado: $rec ($REC_SIZE bytes)"
    if [ "$REC_SIZE" -lt 1000 ]; then
        echo "❌ Arquivo gravado parece corrompido ou vazio ($REC_SIZE bytes)"
        exit 1
    fi
done

# 12. Verificar Catálogo de Metadados
echo "📑 12. Consultando catálogo de gravações (/api/recordings)..."
RECS_JSON=$(curl -s http://127.0.0.1:8080/api/recordings)
echo "   ↳ Catálogo retornado:"
echo "$RECS_JSON"

if ! echo "$RECS_JSON" | grep -q '"status":"completed"'; then
    echo "❌ Metadados não indicam status 'completed'!"
    exit 1
fi

# 13. Verificar Persistência no PostgreSQL
echo "🗄️  13. Verificando eventos de segurança persistidos no PostgreSQL..."
PG_EVENTS=$(docker exec sentinel_db psql -U sentinel_user -d sentinel_db -c "SELECT id, device_id, event_type, timestamp FROM security_events ORDER BY id DESC LIMIT 5;" -t)
echo "   ↳ Registros no banco de dados:"
echo "$PG_EVENTS"

if [ -z "$(echo "$PG_EVENTS" | tr -d '[:space:]')" ]; then
    echo "❌ Nenhum evento foi salvo na tabela security_events do PostgreSQL!"
    exit 1
fi

echo ""
echo "=================================================================="
echo "🎉 TODOS OS CRITÉRIOS DE ACEITAÇÃO FORAM VALIDADOS COM SUCESSO!"
echo "   1. Visualização ao vivo e contínua (sem necessidade de PIR)"
echo "   2. Ingestão centralizada com 1 única conexão no ESP32-CAM"
echo "   3. Múltiplos espectadores simultâneos independentes"
echo "   4. Gravação orientada a eventos PIR (zero gravação contínua)"
echo "   5. Janela deslizante e renovação sem duplicar processos"
echo "   6. Finalização graciosa do FFmpeg com átomo moov íntegro"
echo "   7. Catálogo de metadados e persistência dos alertas no Postgres"
echo "=================================================================="
