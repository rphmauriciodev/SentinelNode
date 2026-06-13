import os
import subprocess

def run_go_generator():
    print("[PlatformIO SCons Hook] Disparando gerador de certificados em Go...")
    
    # Procura pelo arquivo generate_certs.go de forma dinamica (independe de onde o PlatformIO e rodado)
    script_dir = None
    if os.path.exists("generate_certs.go"):
        script_dir = os.path.abspath(".")
    elif os.path.exists("../generate_certs.go"):
        script_dir = os.path.abspath("..")
    elif os.path.exists("../../generate_certs.go"):
        script_dir = os.path.abspath("../..")
        
    if not script_dir:
        print("[PlatformIO SCons Hook ERROR] Nao foi possivel encontrar o arquivo generate_certs.go!")
        raise Exception("Erro de build: generate_certs.go nao encontrado.")
        
    try:
        result = subprocess.run(
            ["go", "run", "generate_certs.go"], 
            cwd=script_dir,
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
