import os
import time
import socket
import struct
import configparser
import threading
from flask import Flask, Response
import cv2
import paho.mqtt.client as mqtt

# Configurações do dispositivo emulado
DEVICE_ID = "esp32-cam-01"
FIRMWARE_MAJOR = 1
FIRMWARE_MINOR = 0
FIRMWARE_PATCH = 0
STREAM_PORT = 8087

app = Flask(__name__)

# Lock para sincronizar acesso à webcam
camera_lock = threading.Lock()
camera = None

def get_camera():
    global camera
    if camera is None:
        # Inicializa a primeira webcam disponível (0 = padrão)
        camera = cv2.VideoCapture(0)
    return camera

def gen_frames():
    cap = get_camera()
    while True:
        with camera_lock:
            success, frame = cap.read()
            if not success:
                # Se falhar ao ler a câmera física, envia um frame cinza de placeholder
                import numpy as np
                frame = np.zeros((480, 640, 3), dtype=np.uint8)
                cv2.putText(frame, "Câmera indisponível no Host", (50, 240), 
                            cv2.FONT_HERSHEY_SIMPLEX, 1, (255, 255, 255), 2)
            
            # Codifica o frame como JPEG
            ret, buffer = cv2.imencode('.jpg', frame, [int(cv2.IMWRITE_JPEG_QUALITY), 80])
            frame_bytes = buffer.tobytes()
            
        yield (b'--123456789000000000000987654321\r\n'
               b'Content-Type: image/jpeg\r\n'
               b'Content-Length: ' + str(len(frame_bytes)).encode() + b'\r\n\r\n' + 
               frame_bytes + b'\r\n')
        time.sleep(0.04)  # ~25 FPS

@app.route('/stream')
def video_stream():
    return Response(gen_frames(), mimetype='multipart/x-mixed-replace; boundary=123456789000000000000987654321')

def get_local_ip():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        # Conexão UDP arbitrária para determinar a interface de rede ativa
        s.connect(('10.255.255.255', 1))
        IP = s.getsockname()[0]
    except Exception:
        IP = '127.0.0.1'
    finally:
        s.close()
    return IP

def encode_heartbeat(device_id, timestamp, status, major, minor, patch, stream_url):
    """
    Serializa o pacote binário conforme definido no protocolo C (protocol.h)
    """
    device_id_bytes = device_id.encode('ascii')
    stream_url_bytes = stream_url.encode('ascii')
    
    # Prefix: Magic1 (0x53), Magic2 (0x4E), Type (0x01 - Heartbeat), Timestamp (uint64), DevId_Len (uint8)
    header = struct.pack(">BBBQB", 0x53, 0x4E, 0x01, timestamp, len(device_id_bytes))
    
    # Middle: Status (uint8), Major (uint8), Minor (uint8), Patch (uint8), Url_Len (uint16)
    middle = struct.pack(">BBBBH", status, major, minor, patch, len(stream_url_bytes))
    
    return header + device_id_bytes + middle + stream_url_bytes

def mqtt_heartbeat_loop(broker_ip, ca_cert, client_cert, client_key, stream_url):
    print(f"[MQTT] Iniciando loop de Heartbeat seguro...")
    
    client = mqtt.Client(client_id="emulator-client")
    
    # Configura TLS/mTLS com os certificados da pasta certs/
    if os.path.exists(ca_cert) and os.path.exists(client_cert) and os.path.exists(client_key):
        print("[MQTT] Carregando certificados TLS mTLS...")
        client.tls_set(
            ca_certs=ca_cert,
            certfile=client_cert,
            keyfile=client_key
        )
        # Ignora verificação de Common Name conforme original do ESP
        client.tls_insecure_set(True)
    else:
        print(f"[MQTT WARNING] Certificados não encontrados nos caminhos informados.")
    
    connected = False
    while not connected:
        try:
            print(f"[MQTT] Conectando ao Broker TLS em mqtts://{broker_ip}:8883 ...")
            client.connect(broker_ip, 8883, keepalive=60)
            connected = True
        except Exception as e:
            print(f"[MQTT ERROR] Falha ao conectar: {e}. Tentando novamente em 5 segundos...")
            time.sleep(5)
            
    client.loop_start()
    
    topic = f"sentinel/devices/{DEVICE_ID}/heartbeat"
    while True:
        try:
            timestamp = int(time.time())
            # Status: 1 (active)
            payload = encode_heartbeat(DEVICE_ID, timestamp, 1, FIRMWARE_MAJOR, FIRMWARE_MINOR, FIRMWARE_PATCH, stream_url)
            
            client.publish(topic, payload, qos=1)
            print(f"[MQTT] Heartbeat binário publicado ({len(payload)} bytes) no tópico: {topic}")
        except Exception as e:
            print(f"[MQTT ERROR] Falha ao publicar heartbeat: {e}")
            
        time.sleep(30) # Envia a cada 30 segundos

if __name__ == '__main__':
    # Lê as configurações originais do private.ini do firmware
    config = configparser.ConfigParser()
    config_path = os.path.join(os.path.dirname(__file__), 'private.ini')
    config.read(config_path)
    
    broker_ip = config.get('private', 'mqtt_broker', fallback='192.168.1.23')
    
    # Caminhos para os certificados do projeto
    certs_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), '../../certs'))
    ca_cert = os.path.join(certs_dir, 'ca.crt')
    client_cert = os.path.join(certs_dir, 'client.crt')
    client_key = os.path.join(certs_dir, 'client.key')
    
    local_ip = get_local_ip()
    stream_url = f"http://{local_ip}:{STREAM_PORT}/stream"
    
    print("\n=======================================================")
    print("EMULADOR SOFTWARE ESP32-CAM ATIVO")
    print(f"IP Local detectado: {local_ip}")
    print(f"Vídeo disponível em: {stream_url}")
    print(f"Broker MQTT IP: {broker_ip}")
    print("=======================================================\n")
    
    # Inicia a thread MQTT do Heartbeat
    mqtt_thread = threading.Thread(
        target=mqtt_heartbeat_loop,
        args=(broker_ip, ca_cert, client_cert, client_key, stream_url),
        daemon=True
    )
    mqtt_thread.start()
    
    # Inicia o servidor Flask para servir o streaming de vídeo
    app.run(host='0.0.0.0', port=STREAM_PORT, debug=False, threaded=True)
