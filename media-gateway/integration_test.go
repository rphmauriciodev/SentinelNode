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
	var camConnections int32

	camServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	t.Run("TESTE 1: Live disponível sem movimento e sem gravação no disco", func(t *testing.T) {
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

	t.Run("TESTE 8: Gravação autônoma sem espectadores conectados", func(t *testing.T) {
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
			t.Fatal("Gravação deveria estar ativa mesmo com 0 espectadores")
		}
	})

	t.Run("TESTE 6 e 7: Múltiplos espectadores e desconexão independente", func(t *testing.T) {

		ctx1, cancel1 := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel1()
		req1, _ := http.NewRequestWithContext(ctx1, http.MethodGet, gwServer.URL+"/live", nil)

		ctx2, cancel2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel2()
		req2, _ := http.NewRequestWithContext(ctx2, http.MethodGet, gwServer.URL+"/live", nil)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			r, err := http.DefaultClient.Do(req1)
			if err == nil {
				buf := make([]byte, 2048)
				for {
					if _, err := r.Body.Read(buf); err != nil {
						break
					}
				}
				r.Body.Close()
			}
		}()
		go func() {
			defer wg.Done()
			r, err := http.DefaultClient.Do(req2)
			if err == nil {
				buf := make([]byte, 2048)
				for {
					if _, err := r.Body.Read(buf); err != nil {
						break
					}
				}
				r.Body.Close()
			}
		}()

		wg.Wait()

		if atomic.LoadInt32(&camConnections) > 1 {
			t.Fatalf("A câmera física excedeu 1 conexão concorrente!")
		}
	})

	t.Run("TESTE 3 e 5: Renovação de tolerância e idempotência de processos", func(t *testing.T) {

		body, _ := json.Marshal(RecordRequest{DeviceID: "esp32-cam-01"})
		resp, _ := http.Post(gwServer.URL+"/record", "application/json", bytes.NewBuffer(body))
		var res map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&res)

		if res["status"] != "recording_extended" {
			t.Fatalf("Esperava status recording_extended, obteve %s", res["status"])
		}

		if !recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria continuar ativa")
		}
	})

	t.Run("TESTE 4: Encerramento correto após expiração da tolerância", func(t *testing.T) {

		time.Sleep(600 * time.Millisecond)

		if recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria ter sido finalizada após expiração da tolerância")
		}

		metas := store.GetAll()
		if len(metas) != 1 || metas[0].Status != "completed" {
			t.Fatalf("Esperava 1 gravação finalizada com sucesso, obteve: %+v", metas)
		}
	})

	t.Run("TESTE 10: Shutdown gracioso", func(t *testing.T) {

		body, _ := json.Marshal(RecordRequest{DeviceID: "esp32-cam-01"})
		_, _ = http.Post(gwServer.URL+"/record", "application/json", bytes.NewBuffer(body))

		if !recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Gravação deveria estar ativa antes do shutdown")
		}

		recCtrl.Shutdown()

		if recCtrl.IsRecording("esp32-cam-01") {
			t.Fatal("Nenhuma gravação pode restar ativa após Shutdown")
		}
	})
}
