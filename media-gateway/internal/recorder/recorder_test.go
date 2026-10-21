package recorder

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func mockExecutor(ctx context.Context, name string, args ...string) *exec.Cmd {

	outPath := args[len(args)-1]

	_ = os.WriteFile(outPath, []byte("mock-mp4-video-data"), 0644)
	return exec.CommandContext(ctx, "cat")
}

func TestRecorder_NoEventsNoRecordings(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMetadataStore(tmpDir)
	rc := NewRecordingController(tmpDir, 100*time.Millisecond, store)
	defer rc.Shutdown()

	if rc.IsRecording("esp32-cam-01") {
		t.Fatal("Esperava estado ocioso (sem gravação) para câmera sem eventos")
	}

	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".mp4" {
			t.Fatalf("Nenhum arquivo MP4 deveria existir, mas encontrou %s", e.Name())
		}
	}
}

func TestRecorder_TriggerStartsRecording(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMetadataStore(tmpDir)
	rc := NewRecordingController(tmpDir, 500*time.Millisecond, store)
	rc.SetCmdExecutor(mockExecutor)
	defer rc.Shutdown()

	status, err := rc.TriggerEvent("esp32-cam-01", "http://127.0.0.1:8080/live/esp32-cam-01")
	if err != nil {
		t.Fatalf("Erro ao disparar evento: %v", err)
	}

	if status != "recording_started" {
		t.Fatalf("Status retornado inesperado: %s", status)
	}

	if !rc.IsRecording("esp32-cam-01") {
		t.Fatal("Esperava que a câmera estivesse em estado de gravação ativa")
	}
}

func TestRecorder_SuccessiveEventsNoDuplicates(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMetadataStore(tmpDir)
	rc := NewRecordingController(tmpDir, 600*time.Millisecond, store)
	rc.SetCmdExecutor(mockExecutor)
	defer rc.Shutdown()

	st1, err := rc.TriggerEvent("esp32-cam-01", "http://127.0.0.1:8080/live/esp32-cam-01")
	if err != nil || st1 != "recording_started" {
		t.Fatalf("1º evento falhou: %v, status=%s", err, st1)
	}

	time.Sleep(100 * time.Millisecond)
	st2, err := rc.TriggerEvent("esp32-cam-01", "http://127.0.0.1:8080/live/esp32-cam-01")
	if err != nil || st2 != "recording_extended" {
		t.Fatalf("2º evento deveria estender a gravação: %v, status=%s", err, st2)
	}

	time.Sleep(100 * time.Millisecond)
	st3, err := rc.TriggerEvent("esp32-cam-01", "http://127.0.0.1:8080/live/esp32-cam-01")
	if err != nil || st3 != "recording_extended" {
		t.Fatalf("3º evento deveria estender a gravação: %v, status=%s", err, st3)
	}

	time.Sleep(800 * time.Millisecond)

	entries, _ := os.ReadDir(tmpDir)
	mp4Count := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".mp4" {
			mp4Count++
		}
	}

	if mp4Count != 1 {
		t.Fatalf("Esperava exatamente 1 arquivo MP4 gerado para eventos sucessivos, mas encontrou %d", mp4Count)
	}
}

func TestRecorder_ToleranceExpiresEndsRecording(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMetadataStore(tmpDir)
	tolerance := 300 * time.Millisecond
	rc := NewRecordingController(tmpDir, tolerance, store)
	rc.SetCmdExecutor(mockExecutor)
	defer rc.Shutdown()

	_, err := rc.TriggerEvent("esp32-cam-01", "http://127.0.0.1:8080/live/esp32-cam-01")
	if err != nil {
		t.Fatalf("Erro ao iniciar gravação: %v", err)
	}

	if !rc.IsRecording("esp32-cam-01") {
		t.Fatal("Esperava gravação ativa")
	}

	time.Sleep(500 * time.Millisecond)

	if rc.IsRecording("esp32-cam-01") {
		t.Fatal("Esperava que a gravação tivesse sido finalizada após expiração da tolerância")
	}

	metas := store.GetAll()
	if len(metas) != 1 || metas[0].Status != "completed" {
		t.Fatalf("Metadados inválidos: esperava 1 gravação com status completed, obteve %+v", metas)
	}
}

func TestRecorder_EventBeforeToleranceKeepsActive(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMetadataStore(tmpDir)
	tolerance := 400 * time.Millisecond
	rc := NewRecordingController(tmpDir, tolerance, store)
	rc.SetCmdExecutor(mockExecutor)
	defer rc.Shutdown()

	_, _ = rc.TriggerEvent("esp32-cam-01", "http://127.0.0.1:8080/live/esp32-cam-01")

	time.Sleep(250 * time.Millisecond)
	st, _ := rc.TriggerEvent("esp32-cam-01", "http://127.0.0.1:8080/live/esp32-cam-01")
	if st != "recording_extended" {
		t.Fatalf("Esperava recording_extended, obteve %s", st)
	}

	time.Sleep(200 * time.Millisecond)
	if !rc.IsRecording("esp32-cam-01") {
		t.Fatal("A gravação deveria continuar ativa em t=450ms pois o timer foi reiniciado em t=250ms!")
	}

	time.Sleep(300 * time.Millisecond)
	if rc.IsRecording("esp32-cam-01") {
		t.Fatal("A gravação deveria ter finalizado após o fim do temporizador")
	}
}

func TestRecorder_ShutdownGracefullyStopsRecordings(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewMetadataStore(tmpDir)
	rc := NewRecordingController(tmpDir, 10*time.Second, store)
	rc.SetCmdExecutor(mockExecutor)

	_, _ = rc.TriggerEvent("esp32-cam-01", "http://127.0.0.1:8080/live/esp32-cam-01")
	if !rc.IsRecording("esp32-cam-01") {
		t.Fatal("Esperava gravação ativa antes do shutdown")
	}

	rc.Shutdown()

	if rc.IsRecording("esp32-cam-01") {
		t.Fatal("Todas as gravações deveriam estar finalizadas após rc.Shutdown()")
	}

	metas := store.GetAll()
	if len(metas) != 1 || metas[0].Status != "completed" {
		t.Fatalf("Gravação deveria ter sido salva como completed após shutdown controlado")
	}
}
