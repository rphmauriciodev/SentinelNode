package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rphmauriciodev/SentinelNode/media-gateway/internal/recorder"
	"github.com/rphmauriciodev/SentinelNode/media-gateway/internal/stream"
)

func createSampleJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func mockExecutorForIntegration(ctx context.Context, name string, args ...string) *exec.Cmd {
	outPath := args[len(args)-1]
	_ = os.WriteFile(outPath, []byte("valid-mp4-test-data"), 0644)
	return exec.CommandContext(ctx, "cat")
}

func TestIntegration_SentinelNodeScenarios(t *testing.T) {
	tmpDir := t.TempDir()
	frame := createSampleJPEG(32, 32)
	var camOnline int32 = 1
	var camConnections int32

	camServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&camOnline) == 0 {
			http.Error(w, "Camera offline", http.StatusServiceUnavailable)
			return
		}

		count := atomic.AddInt32(&camConnections, 1)
		defer atomic.AddInt32(&camConnections, -1)

		if count > 1 {
			http.Error(w, "Only one stream client allowed", http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=123456789000000000000987654321")
		flusher, _ := w.(http.Flusher)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if atomic.LoadInt32(&camOnline) == 0 {
					return
				}
				fmt.Fprintf(w, "--123456789000000000000987654321\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(frame))
				w.Write(frame)
				fmt.Fprintf(w, "\r\n")
				flusher.Flush()
			}
		}
	}))
	defer camServer.Close()

	store := recorder.NewMetadataStore(tmpDir)
	tolerance := 400 * time.Millisecond
	recCtrl := recorder.NewRecordingController(tmpDir, tolerance, store)
	recCtrl.SetCmdExecutor(mockExecutorForIntegration)
	strMgr := stream.NewStreamManager()
	defer strMgr.Close()

	cs := strMgr.Register("esp32-cam-01", camServer.URL)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cs.Status()["status"] == "streaming" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		cs.ServeLiveHTTP(w, r)
	})
	mux.HandleFunc("/record", func(w http.ResponseWriter, r *http.Request) {
		var req RecordRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		proxyURL := fmt.Sprintf("http://127.0.0.1/live?device_id=%s", req.DeviceID)
		st, err := recCtrl.TriggerEvent(req.DeviceID, proxyURL)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": st})
	})
	mux.HandleFunc("/api/recordings", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(store.GetAll())
	})

	gwServer := httptest.NewServer(mux)
	defer gwServer.Close()

	t.Run("TESTE 1: Sem eventos de movimento, o vídeo continua disponível, mas nenhuma gravação é persistida", func(t *testing.T) {
		ctxLive, cancelLive := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancelLive()

		req, _ := http.NewRequestWithContext(ctxLive, http.MethodGet, gwServer.URL+"/live", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Falha ao abrir stream ao vivo: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Status HTTP inesperado para live: %d", resp.StatusCode)
		}

		if recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Nenhuma gravação deveria estar ativa sem evento PIR")
		}
		if len(store.GetAll()) > 0 {
			t.Fatal("Nenhum arquivo de vídeo deveria estar registrado")
		}
	})

	t.Run("TESTE 2: Ao receber um evento PIR válido, a gravação é iniciada", func(t *testing.T) {
		body, _ := json.Marshal(RecordRequest{DeviceID: "esp32-cam-01"})
		resp, err := http.Post(gwServer.URL+"/record", "application/json", bytes.NewBuffer(body))
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("Falha ao disparar gravação: %v", err)
		}
		var res map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&res)
		if res["status"] != "recording_started" {
			t.Fatalf("Esperava status recording_started, obteve %s", res["status"])
		}

		if !recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria estar ativa após evento PIR válido")
		}
	})

	t.Run("TESTE 3: Eventos sucessivos durante a gravação não criam processos ou arquivos duplicados indevidamente", func(t *testing.T) {
		body, _ := json.Marshal(RecordRequest{DeviceID: "esp32-cam-01"})
		resp, err := http.Post(gwServer.URL+"/record", "application/json", bytes.NewBuffer(body))
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("Falha ao disparar evento sucessivo: %v", err)
		}
		var res map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&res)

		if res["status"] != "recording_extended" {
			t.Fatalf("Esperava status recording_extended, obteve %s", res["status"])
		}

		if !recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria continuar ativa sem interrupção")
		}

		// Metadados ainda devem apontar apenas para a gravação atual
		metas := store.GetAll()
		if len(metas) != 1 {
			t.Fatalf("Esperava exatamente 1 registro de gravação ativa, obteve %d", len(metas))
		}
	})

	t.Run("TESTE 4: Sem novos eventos durante o período de tolerância, a gravação é finalizada", func(t *testing.T) {
		time.Sleep(tolerance + 200*time.Millisecond)

		if recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria ter sido finalizada após expiração da tolerância")
		}

		metas := store.GetAll()
		if len(metas) != 1 || metas[0].Status != "completed" {
			t.Fatalf("Esperava 1 gravação finalizada com sucesso, obteve: %+v", metas)
		}
	})

	t.Run("TESTE 5: Um novo evento antes do término da tolerância reinicia o temporizador e mantém a gravação ativa", func(t *testing.T) {
		body, _ := json.Marshal(RecordRequest{DeviceID: "esp32-cam-01"})
		resp, _ := http.Post(gwServer.URL+"/record", "application/json", bytes.NewBuffer(body))
		var res map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&res)
		if res["status"] != "recording_started" {
			t.Fatalf("Esperava recording_started para nova sessão, obteve %s", res["status"])
		}

		// Aguarda mais de 50% da tolerância (250ms de 400ms)
		time.Sleep(250 * time.Millisecond)

		// Dispara novo evento que deve renovar a janela por mais 400ms
		resp2, _ := http.Post(gwServer.URL+"/record", "application/json", bytes.NewBuffer(body))
		_ = json.NewDecoder(resp2.Body).Decode(&res)
		if res["status"] != "recording_extended" {
			t.Fatalf("Esperava recording_extended na renovação, obteve %s", res["status"])
		}

		// Aguarda mais 250ms (total 500ms > tolerância inicial de 400ms)
		time.Sleep(250 * time.Millisecond)

		// A gravação AINDA deve estar ativa pois a tolerância foi renovada!
		if !recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria continuar ativa devido à renovação da tolerância")
		}

		// Aguarda expirar a nova tolerância
		time.Sleep(300 * time.Millisecond)
		if recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria ter sido finalizada após expiração da nova tolerância")
		}
	})

	t.Run("TESTE 5B: Presença contínua mantida pelo PIR não interrompe a gravação enquanto houver detecção sustentada", func(t *testing.T) {
		body, _ := json.Marshal(RecordRequest{DeviceID: "esp32-cam-01"})
		resp, _ := http.Post(gwServer.URL+"/record", "application/json", bytes.NewBuffer(body))
		var res map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&res)
		if res["status"] != "recording_started" {
			t.Fatalf("Esperava recording_started para nova sessão, obteve %s", res["status"])
		}

		for i := 1; i <= 4; i++ {
			time.Sleep(200 * time.Millisecond)
			respLoop, _ := http.Post(gwServer.URL+"/record", "application/json", bytes.NewBuffer(body))
			var resLoop map[string]string
			_ = json.NewDecoder(respLoop.Body).Decode(&resLoop)
			if resLoop["status"] != "recording_extended" {
				t.Fatalf("Esperava recording_extended na renovação %d, obteve %s", i, resLoop["status"])
			}
			if !recCtrl.IsRecording("esp32-cam-01") {
				t.Fatalf("Gravação deveria continuar ativa durante presença contínua (iteração %d)", i)
			}
		}

		time.Sleep(200 * time.Millisecond)
		if !recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria continuar ativa dentro da tolerância renovada")
		}

		time.Sleep(300 * time.Millisecond)
		if recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria ter sido finalizada após cessar a presença e expirar a tolerância")
		}
	})

	t.Run("TESTE 6: Dois ou mais espectadores conseguem acessar o mesmo fluxo sem depender de conexões independentes com a câmera", func(t *testing.T) {
		ctx1, cancel1 := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel1()
		req1, _ := http.NewRequestWithContext(ctx1, http.MethodGet, gwServer.URL+"/live", nil)

		ctx2, cancel2 := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel2()
		req2, _ := http.NewRequestWithContext(ctx2, http.MethodGet, gwServer.URL+"/live", nil)

		ctx3, cancel3 := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel3()
		req3, _ := http.NewRequestWithContext(ctx3, http.MethodGet, gwServer.URL+"/live", nil)

		var wg sync.WaitGroup
		wg.Add(3)
		for _, r := range []*http.Request{req1, req2, req3} {
			go func(req *http.Request) {
				defer wg.Done()
				resp, err := http.DefaultClient.Do(req)
				if err == nil {
					buf := make([]byte, 2048)
					for {
						if _, err := resp.Body.Read(buf); err != nil {
							break
						}
					}
					resp.Body.Close()
				}
			}(r)
		}
		wg.Wait()

		// Invariante essencial: NUNCA abrir mais de 1 conexão direta com a câmera
		if atomic.LoadInt32(&camConnections) > 1 {
			t.Fatalf("Violação de invariante: A câmera física recebeu mais de 1 conexão simultânea!")
		}
	})

	t.Run("TESTE 7: A desconexão de um espectador não interrompe a gravação nem os demais espectadores", func(t *testing.T) {
		// Inicia gravação
		body, _ := json.Marshal(RecordRequest{DeviceID: "esp32-cam-01"})
		_, _ = http.Post(gwServer.URL+"/record", "application/json", bytes.NewBuffer(body))
		if !recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria estar ativa")
		}

		ctxViewerA, cancelViewerA := context.WithCancel(context.Background())
		ctxViewerB, cancelViewerB := context.WithTimeout(context.Background(), 350*time.Millisecond)
		defer cancelViewerB()

		viewerBReceivedFrames := int32(0)

		var wg sync.WaitGroup
		wg.Add(2)

		// Espectador A: desconecta abruptamente após 80ms
		go func() {
			defer wg.Done()
			reqA, _ := http.NewRequestWithContext(ctxViewerA, http.MethodGet, gwServer.URL+"/live", nil)
			respA, err := http.DefaultClient.Do(reqA)
			if err == nil {
				go func() {
					time.Sleep(80 * time.Millisecond)
					cancelViewerA()
				}()
				buf := make([]byte, 2048)
				for {
					if _, err := respA.Body.Read(buf); err != nil {
						break
					}
				}
				respA.Body.Close()
			}
		}()

		// Espectador B: continua consumindo sem interrupção
		go func() {
			defer wg.Done()
			reqB, _ := http.NewRequestWithContext(ctxViewerB, http.MethodGet, gwServer.URL+"/live", nil)
			respB, err := http.DefaultClient.Do(reqB)
			if err == nil {
				buf := make([]byte, 2048)
				for {
					n, err := respB.Body.Read(buf)
					if n > 0 {
						atomic.AddInt32(&viewerBReceivedFrames, 1)
					}
					if err != nil {
						break
					}
				}
				respB.Body.Close()
			}
		}()

		wg.Wait()

		if atomic.LoadInt32(&viewerBReceivedFrames) == 0 {
			t.Fatal("Espectador B deveria ter continuado recebendo frames normalmente")
		}

		if !recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("A gravação não pode ser interrompida pela desconexão de espectadores")
		}

		_ = recCtrl.StopSession("esp32-cam-01")
	})

	t.Run("TESTE 8: A gravação continua funcionando mesmo sem espectadores", func(t *testing.T) {
		body, _ := json.Marshal(RecordRequest{DeviceID: "esp32-cam-01"})
		resp, err := http.Post(gwServer.URL+"/record", "application/json", bytes.NewBuffer(body))
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("Falha ao disparar gravação autônoma: %v", err)
		}

		if !recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria estar ativa autonomamente sem espectadores")
		}

		time.Sleep(tolerance + 150*time.Millisecond)
		if recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria ter sido finalizada após tolerância")
		}
	})

	t.Run("TESTE 9: A desconexão da câmera é identificada e tratada", func(t *testing.T) {
		// Simula queda da câmera física
		atomic.StoreInt32(&camOnline, 0)

		// Aguarda o leitor detectar a desconexão (timeout/EOF)
		deadlineOffline := time.Now().Add(2 * time.Second)
		isOffline := false
		for time.Now().Before(deadlineOffline) {
			if cs.Status()["status"] == "offline" {
				isOffline = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		if !isOffline {
			t.Fatalf("Esperava que o status da câmera transitasse para 'offline' após queda da conexão")
		}

		// Restaura a câmera física
		atomic.StoreInt32(&camOnline, 1)

		// Aguarda o gateway restabelecer a conexão de ingestão
		deadlineOnline := time.Now().Add(3 * time.Second)
		isStreaming := false
		for time.Now().Before(deadlineOnline) {
			if cs.Status()["status"] == "streaming" {
				isStreaming = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		if !isStreaming {
			t.Fatalf("Esperava que o status da câmera retornasse para 'streaming' após recuperação da conexão")
		}
	})

	t.Run("TESTE 10: O encerramento do backend finaliza ou interrompe os processos de gravação de maneira controlada", func(t *testing.T) {
		body, _ := json.Marshal(RecordRequest{DeviceID: "esp32-cam-01"})
		_, _ = http.Post(gwServer.URL+"/record", "application/json", bytes.NewBuffer(body))

		if !recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria estar ativa antes do shutdown")
		}

		recCtrl.Shutdown()

		if recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Nenhuma gravação pode restar ativa após Shutdown controlado")
		}
	})
}
