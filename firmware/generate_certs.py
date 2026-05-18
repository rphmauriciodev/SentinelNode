import subprocess
import os

def run_go_generator():
    print("[PlatformIO SCons Hook] Disparando gerador de certificados em Go...")
    try:
        result = subprocess.run(
            ["go", "run", "generate_certs.go"], 
            capture_output=True, 
            text=True, 
            check=True
        )
        print(result.stdout)
    except subprocess.CalledProcessError as e:
        print("[PlatformIO SCons Hook ERROR] Falha ao executar generate_certs.go!")
        print(e.stderr)
        raise Exception("Erro de build: Falha na geracao automatica de certificados em Go.")

run_go_generator()
