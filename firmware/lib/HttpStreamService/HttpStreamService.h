#ifndef HTTP_STREAM_SERVICE_H
#define HTTP_STREAM_SERVICE_H

#include <Arduino.h>
#include "esp_http_server.h"
#include "esp_camera.h"

class HttpStreamService {
public:
    // Inicia o servidor HTTP na porta 80 e registra o endpoint de stream
    static bool start();
    
    // Para o servidor HTTP e limpa os sockets abertos
    static void stop();
    
    // Verifica se existe algum cliente consumindo o stream de vídeo ativamente
    static bool isStreaming();

private:
    // Manipulador do loop de streaming MJPEG (C-Style callback para o httpd)
    static esp_err_t streamHandler(httpd_req_t* req);

    static httpd_handle_t _serverHandle;
    static volatile bool _isStreamingActive;
};

#endif // HTTP_STREAM_SERVICE_H
