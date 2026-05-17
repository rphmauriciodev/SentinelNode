set -e

echo "🔑 1. Gerando a Autoridade Certificadora Root (CA)..."
openssl genrsa -out ca.key 2048
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt -subj "/CN=SentinelNodeRootCA"

echo "🔑 2. Gerando chaves e certificados para o Broker MQTT (Server)..."
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr -subj "/CN=localhost"
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 365 -sha256

echo "🔑 3. Gerando chaves e certificados para os Clientes Go (Câmera e Backend)..."
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr -subj "/CN=sentinel-client"
openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client.crt -days 365 -sha256

chmod 644 ca.crt server.crt client.crt
chmod 644 server.key client.key

echo "✅ Todos os certificados foram gerados com sucesso na pasta certs/!"
