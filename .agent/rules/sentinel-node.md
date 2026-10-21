# REGRAS DE DESENVOLVIMENTO - SENTINELNODE

1. **Preservação do Broadcaster Central:**
   * Nunca conecte FFmpeg ou visualizadores diretamente ao endereço do ESP32-CAM.
   * Sempre passe pelo `StreamManager` do `media-gateway` via endpoint proxy `http://127.0.0.1:8080/live?device_id=...`.

2. **Gerenciamento de Gravação e Concorrência:**
   * Não altere a lógica de sliding window sem justificativa comprovada.
   * Eventos PIR sucessivos devem reiniciar o temporizador de tolerância (`30s`), mantendo o processo de gravação ativo.
   * O encerramento do processo FFmpeg deve sempre enviar `q` para o `stdin` antes de qualquer sinal de término abrupto (`SIGINT`/`SIGKILL`), para evitar que o arquivo MP4 fique corrompido sem o átomo `moov`.

3. **Compatibilidade com Protocolo Binário:**
   * A integridade dos Magic Numbers (`0x53 0x4E`) e o alinhamento de bytes Big-Endian devem ser rigorosamente preservados em qualquer alteração nos pacotes de rede.

4. **Sem Dependências Desnecessárias:**
   * O dashboard web deve permanecer leve e servido diretamente pelo Go, sem introduzir frameworks frontend pesados (`node_modules`, `webpack`, etc.).

5. **Testes Contínuos:**
   * Qualquer alteração no `media-gateway` ou `ingestion-engine` deve passar em todos os testes (`go test -v ./...`).
