#!/bin/sh
set -e

mkdir -p /certs
cd /certs

if [ ! -f /certs/ca.crt ] || [ ! -f /certs/server.crt ] || [ ! -f /certs/client.crt ]; then
    echo "🔑 Certificados mTLS não encontrados em /certs. Gerando novos certificados..."
    /usr/local/bin/generate_certs.sh
    echo "✅ Certificados mTLS gerados com sucesso em /certs!"
else
    echo "🔑 Certificados mTLS já existem em /certs. Nenhuma ação necessária."
fi
