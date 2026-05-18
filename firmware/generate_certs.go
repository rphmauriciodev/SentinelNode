package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

func main() {
	certsDir, err := filepath.Abs(filepath.Join("..", "certs"))
	if err != nil {
		fmt.Printf("[Go Certs Script ERROR] %v\n", err)
		os.Exit(1)
	}
	includeDir, err := filepath.Abs("include")
	if err != nil {
		fmt.Printf("[Go Certs Script ERROR] %v\n", err)
		os.Exit(1)
	}
	secretsHeader := filepath.Join(includeDir, "secrets.h")

	err = os.MkdirAll(includeDir, os.ModePerm)
	if err != nil {
		fmt.Printf("[Go Certs Script ERROR] Falha ao criar pasta include/: %v\n", err)
		os.Exit(1)
	}

	caPath := filepath.Join(certsDir, "ca.crt")
	clientPath := filepath.Join(certsDir, "client.crt")
	keyPath := filepath.Join(certsDir, "client.key")

	readFileOrDefault := func(path string, defaultLabel string) string {
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("[Go Certs Script] Lendo: %s\n", path)
			content, err := ioutil.ReadFile(path)
			if err == nil {
				return string(content)
			}
		}
		fmt.Printf("[Go Certs Script WARNING] Arquivo nao encontrado: %s\n", path)
		return fmt.Sprintf("// ERRO: Arquivo %s nao encontrado. Execute o script de geracao de certificados.", defaultLabel)
	}

	caContent := readFileOrDefault(caPath, "ca.crt")
	clientContent := readFileOrDefault(clientPath, "client.crt")
	keyContent := readFileOrDefault(keyPath, "client.key")

	headerContent := fmt.Sprintf(`#ifndef SECRETS_H
#define SECRETS_H

// =========================================================================
// ATENCAO: ESTE ARQUIVO E GERADO AUTOMATICAMENTE PELO SCRIPT generate_certs.go
// NAO MODIFIQUE ESTE ARQUIVO DIRETAMENTE! SEUS DADOS SERAO OVERWRITTEN.
// =========================================================================

const char* ca_cert = R"EOF(
%s)EOF";

const char* client_cert = R"EOF(
%s)EOF";

const char* client_key = R"EOF(
%s)EOF";

#endif // SECRETS_H
`, caContent, clientContent, keyContent)

	err = ioutil.WriteFile(secretsHeader, []byte(headerContent), 0644)
	if err != nil {
		fmt.Printf("[Go Certs Script ERROR] Falha ao escrever secrets.h: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[Go Certs Script SUCCESS] secrets.h gerado com sucesso em %s!\n", secretsHeader)
}
