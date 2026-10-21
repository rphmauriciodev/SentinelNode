import os
import re
import time
import socket
import struct
import threading
from flask import Flask, Response, jsonify
import cv2
import paho.mqtt.client as mqtt

DEVICE_ID = "esp32-cam-01"
FIRMWARE_MAJOR = 1
FIRMWARE_MINOR = 0
FIRMWARE_PATCH = 0
STREAM_PORT = 8087

app = Flask(__name__)

camera_lock = threading.Lock()
camera = None

streaming_active = False
streaming_lock = threading.Lock()

mqtt_client = None

def get_camera():
    global camera
    if camera is None:
        camera = cv2.VideoCapture(0)
    return camera

def gen_frames():
    cap = get_camera()
    while True:
        with camera_lock:
            success, frame = cap.read()
            if not success:
                import numpy as np
                frame = np.zeros((480, 640, 3), dtype=np.uint8)
                cv2.putText(frame, "Camera indisponivel no Host", (50, 240), 
                            cv2.FONT_HERSHEY_SIMPLEX, 1, (255, 255, 255), 2)
            
            ret, buffer = cv2.imencode('.jpg', frame, [int(cv2.IMWRITE_JPEG_QUALITY), 80])
            frame_bytes = buffer.tobytes()
            
        yield (b'--123456789000000000000987654321\r\n'
               b'Content-Type: image/jpeg\r\n'
               b'Content-Length: ' + str(len(frame_bytes)).encode() + b'\r\n\r\n' + 
               frame_bytes + b'\r\n')
        time.sleep(0.04)

@app.route('/stream')
def video_stream():
    global streaming_active
    with streaming_lock:
        if streaming_active:
            return Response("Only one stream client allowed", status=403)
        streaming_active = True

    def stream_gen():
        global streaming_active
        try:
            yield from gen_frames()
        finally:
            with streaming_lock:
                streaming_active = False

    return Response(stream_gen(), mimetype='multipart/x-mixed-replace; boundary=123456789000000000000987654321')

@app.route('/pir/trigger', methods=['GET', 'POST'])
def trigger_pir():
    global mqtt_client
    if not mqtt_client:
        return jsonify({"status": "error", "message": "MQTT offline"}), 503

    local_ip = get_local_ip()
    stream_url = f"http://{local_ip}:{STREAM_PORT}/stream"
    timestamp = int(time.time())
    payload = encode_event(DEVICE_ID, timestamp, 1, stream_url)

    topic = f"sentinel/devices/{DEVICE_ID}/events"
    mqtt_client.publish(topic, payload, qos=1)
    return jsonify({"status": "motion_event_published", "topic": topic, "device_id": DEVICE_ID})

def get_local_ip():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        s.connect(('10.255.255.255', 1))
        IP = s.getsockname()[0]
    except Exception:
        IP = '127.0.0.1'
    finally:
        s.close()
    return IP

def encode_heartbeat(device_id, timestamp, status, major, minor, patch, stream_url):
    device_id_bytes = device_id.encode('ascii')
    stream_url_bytes = stream_url.encode('ascii')
    
    header = struct.pack(">BBBQB", 0x53, 0x4E, 0x01, timestamp, len(device_id_bytes))
    middle = struct.pack(">BBBBH", status, major, minor, patch, len(stream_url_bytes))
    
    return header + device_id_bytes + middle + stream_url_bytes

def encode_event(device_id, timestamp, event_type, video_url=""):
    device_id_bytes = device_id.encode('ascii')
    video_url_bytes = video_url.encode('ascii')

    header = struct.pack(">BBBQB", 0x53, 0x4E, 0x02, timestamp, len(device_id_bytes))
    middle = struct.pack(">BH", event_type, len(video_url_bytes))

    return header + device_id_bytes + middle + video_url_bytes

def resolve_broker_ip():
    env_ip = os.environ.get("MQTT_BROKER_IP")
    if env_ip:
        return env_ip

    search_paths = [
        os.path.join(os.path.dirname(__file__), 'private.ini'),
        os.path.join(os.path.dirname(__file__), '..', 'private.ini'),
        os.path.join(os.path.dirname(__file__), '..', '..', 'firmware', 'private.ini'),
    ]

    for p in search_paths:
        if os.path.exists(p):
            try:
                with open(p, 'r') as fp:
                    content = fp.read()
                m = re.search(r'MQTT_BROKER_IP\\?=\\?"([^\\"]+)\\"', content)
                if m:
                    return m.group(1)
                m2 = re.search(r'mqtt_broker\s*=\s*([^\s]+)', content)
                if m2:
                    return m2.group(1)
            except Exception:
                pass

    return "127.0.0.1"

def create_mqtt_client():
    try:
        return mqtt.Client(mqtt.CallbackAPIVersion.VERSION1, client_id="emulator-client")
    except (AttributeError, TypeError):
        return mqtt.Client(client_id="emulator-client")

def mqtt_heartbeat_loop(broker_ip, ca_cert, client_cert, client_key, stream_url):
    global mqtt_client
    print(f"[MQTT] Iniciando loop de Heartbeat seguro...")

    client = create_mqtt_client()
    mqtt_client = client

    if os.path.exists(ca_cert) and os.path.exists(client_cert) and os.path.exists(client_key):
        print("[MQTT] Carregando certificados TLS mTLS...")
        client.tls_set(
            ca_certs=ca_cert,
            certfile=client_cert,
            keyfile=client_key
        )
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
            payload = encode_heartbeat(DEVICE_ID, timestamp, 1, FIRMWARE_MAJOR, FIRMWARE_MINOR, FIRMWARE_PATCH, stream_url)
            client.publish(topic, payload, qos=1)
            print(f"[MQTT] Heartbeat binário publicado ({len(payload)} bytes) no tópico: {topic}")
        except Exception as e:
            print(f"[MQTT ERROR] Falha ao publicar heartbeat: {e}")

        time.sleep(30)

if __name__ == '__main__':
    broker_ip = resolve_broker_ip()

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
    print(f"Trigger PIR: http://{local_ip}:{STREAM_PORT}/pir/trigger")
    print("=======================================================\n")

    mqtt_thread = threading.Thread(
        target=mqtt_heartbeat_loop,
        args=(broker_ip, ca_cert, client_cert, client_key, stream_url),
        daemon=True
    )
    mqtt_thread.start()

    app.run(host='0.0.0.0', port=STREAM_PORT, debug=False, threaded=True)
