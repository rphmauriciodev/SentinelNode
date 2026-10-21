package stream

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func createTestJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func TestFrameReader_WithContentLength(t *testing.T) {
	frame1 := createTestJPEG(64, 48)
	frame2 := createTestJPEG(64, 48)

	var streamBuf bytes.Buffer
	boundary := "123456789000000000000987654321"

	fmt.Fprintf(&streamBuf, "--%s\r\n", boundary)
	fmt.Fprintf(&streamBuf, "Content-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(frame1))
	streamBuf.Write(frame1)
	fmt.Fprintf(&streamBuf, "\r\n--%s\r\n", boundary)
	fmt.Fprintf(&streamBuf, "Content-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(frame2))
	streamBuf.Write(frame2)
	fmt.Fprintf(&streamBuf, "\r\n")

	reader := NewFrameReader(&streamBuf, boundary)

	f1, err := reader.NextFrame()
	if err != nil {
		t.Fatalf("falha ao ler frame 1: %v", err)
	}
	if len(f1) != len(frame1) {
		t.Errorf("tamanho do frame 1 diferente: esperado %d, obteve %d", len(frame1), len(f1))
	}
	if !bytes.Equal(f1, frame1) {
		t.Errorf("bytes do frame 1 não coincidem")
	}

	f2, err := reader.NextFrame()
	if err != nil {
		t.Fatalf("falha ao ler frame 2: %v", err)
	}
	if len(f2) != len(frame2) {
		t.Errorf("tamanho do frame 2 diferente: esperado %d, obteve %d", len(frame2), len(f2))
	}
	if !bytes.Equal(f2, frame2) {
		t.Errorf("bytes do frame 2 não coincidem")
	}
}

func TestBroadcaster_MultipleSubscribersAndSlowConsumer(t *testing.T) {
	b := NewBroadcaster(5)
	defer b.Close()

	ch1, unsub1 := b.Subscribe()
	defer unsub1()

	ch2, unsub2 := b.Subscribe()
	defer unsub2()

	frame := createTestJPEG(32, 32)

	b.Broadcast(frame)

	select {
	case f := <-ch1:
		if !bytes.Equal(f, frame) {
			t.Errorf("ch1 recebeu frame incorreto")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ch1 não recebeu o frame a tempo")
	}

	select {
	case f := <-ch2:
		if !bytes.Equal(f, frame) {
			t.Errorf("ch2 recebeu frame incorreto")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ch2 não recebeu o frame a tempo")
	}

	for i := 0; i < 20; i++ {
		b.Broadcast(frame)
	}

	if len(ch2) > 5 {
		t.Errorf("ch2 buffer excedeu o tamanho máximo configurado: %d", len(ch2))
	}
}

func TestMultipleViewers_SingleCameraConnection(t *testing.T) {
	var cameraConnections int32
	frame := createTestJPEG(32, 32)

	camServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		curr := atomic.AddInt32(&cameraConnections, 1)
		defer atomic.AddInt32(&cameraConnections, -1)

		if curr > 1 {
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

	sm := NewStreamManager()
	defer sm.Close()

	cs := sm.Register("esp32-cam-01", camServer.URL)

	requireStreamingStatus(t, cs, 3*time.Second)

	var wg sync.WaitGroup
	var framesReceived int64
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/live/esp32-cam-01", nil).WithContext(ctx)

			cs.ServeLiveHTTP(rec, req)

			if rec.Body.Len() > 0 {
				atomic.AddInt64(&framesReceived, 1)
			}
		}(i)
	}

	wg.Wait()

	if maxConn := atomic.LoadInt32(&cameraConnections); maxConn > 1 {
		t.Fatalf("ERRO: A câmera recebeu %d conexões simultâneas (máximo permitido é 1)!", maxConn)
	}

	if atomic.LoadInt64(&framesReceived) < 3 {
		t.Fatalf("Esperava que todos os 3 espectadores recebessem dados, mas apenas %d receberam", framesReceived)
	}
}

func TestViewerDisconnect_DoesNotAffectOthers(t *testing.T) {
	frame := createTestJPEG(32, 32)

	camServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	sm := NewStreamManager()
	defer sm.Close()

	cs := sm.Register("esp32-cam-01", camServer.URL)
	requireStreamingStatus(t, cs, 3*time.Second)

	ctx1, cancel1 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel1()
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/live/esp32-cam-01", nil).WithContext(ctx1)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel2()
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/live/esp32-cam-01", nil).WithContext(ctx2)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		cs.ServeLiveHTTP(rec1, req1)
	}()
	go func() {
		defer wg.Done()
		cs.ServeLiveHTTP(rec2, req2)
	}()

	wg.Wait()

	if rec2.Body.Len() <= rec1.Body.Len() {
		t.Fatalf("Viewer 2 deveria ter continuado recebendo após Viewer 1 desconectar! (V1: %d B, V2: %d B)",
			rec1.Body.Len(), rec2.Body.Len())
	}
}

func TestCameraDisconnect_DetectionAndRecovery(t *testing.T) {
	frame := createTestJPEG(32, 32)
	var serveActive int32 = 1

	camServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&serveActive) == 0 {
			http.Error(w, "Camera Offline", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=123456789000000000000987654321")
		flusher, _ := w.(http.Flusher)

		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if atomic.LoadInt32(&serveActive) == 0 {
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

	sm := NewStreamManager()
	defer sm.Close()

	cs := sm.Register("esp32-cam-01", camServer.URL)
	requireStreamingStatus(t, cs, 3*time.Second)

	atomic.StoreInt32(&serveActive, 0)

	deadline := time.Now().Add(3 * time.Second)
	offlineDetected := false
	for time.Now().Before(deadline) {
		st := cs.Status()
		if st["status"] == "offline" || st["status"] == "connecting" {
			offlineDetected = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !offlineDetected {
		t.Fatalf("Falha: Desconexão da câmera não foi detectada pelo sistema!")
	}

	atomic.StoreInt32(&serveActive, 1)

	requireStreamingStatus(t, cs, 4*time.Second)
}

func requireStreamingStatus(t *testing.T, cs *CameraStream, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st := cs.Status()
		if st["status"] == "streaming" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("Timeout aguardando status 'streaming' para %s (atual: %v)", cs.DeviceID(), cs.Status())
}
