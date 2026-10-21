package stream

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"log"
	"mime"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type CameraStream struct {
	deviceID    string
	sourceURL   string
	broadcaster *Broadcaster

	mu        sync.RWMutex
	status    string
	lastError string
	lastSeen  time.Time

	frameCount   uint64
	currentFPS   float64
	lastFPSTime  time.Time
	lastFPSCount uint64

	ctx    context.Context
	cancel context.CancelFunc
}

func NewCameraStream(deviceID, sourceURL string) *CameraStream {
	ctx, cancel := context.WithCancel(context.Background())
	cs := &CameraStream{
		deviceID:    deviceID,
		sourceURL:   sourceURL,
		broadcaster: NewBroadcaster(10),
		status:      "connecting",
		lastFPSTime: time.Now(),
		ctx:         ctx,
		cancel:      cancel,
	}

	go cs.runIngestion()
	return cs
}

func (cs *CameraStream) UpdateSourceURL(newURL string) {
	cs.mu.Lock()
	if cs.sourceURL == newURL {
		cs.mu.Unlock()
		return
	}
	cs.sourceURL = newURL
	cs.cancel()

	cs.ctx, cs.cancel = context.WithCancel(context.Background())
	cs.mu.Unlock()

	go cs.runIngestion()
}

func (cs *CameraStream) Stop() {
	cs.cancel()
	cs.broadcaster.Close()
	cs.mu.Lock()
	cs.status = "offline"
	cs.mu.Unlock()
}

func (cs *CameraStream) setStatus(status, lastError string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.status = status
	cs.lastError = lastError
	if status == "streaming" {
		cs.lastSeen = time.Now()
	}
}

func (cs *CameraStream) Status() map[string]interface{} {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return map[string]interface{}{
		"device_id":      cs.deviceID,
		"source_url":     cs.sourceURL,
		"status":         cs.status,
		"last_error":     cs.lastError,
		"last_seen":      cs.lastSeen.Format(time.RFC3339),
		"fps":            cs.currentFPS,
		"frame_count":    atomic.LoadUint64(&cs.frameCount),
		"active_viewers": cs.broadcaster.SubscribersCount(),
	}
}

func (cs *CameraStream) DeviceID() string {
	return cs.deviceID
}

func (cs *CameraStream) SourceURL() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.sourceURL
}

func (cs *CameraStream) Broadcaster() *Broadcaster {
	return cs.broadcaster
}

func (cs *CameraStream) runIngestion() {
	backoff := 1 * time.Second
	maxBackoff := 10 * time.Second

	client := &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}

	for {
		select {
		case <-cs.ctx.Done():
			return
		default:
		}

		cs.mu.RLock()
		url := cs.sourceURL
		cs.mu.RUnlock()

		if url == "" {
			cs.setStatus("offline", "empty source URL")
			time.Sleep(1 * time.Second)
			continue
		}

		cs.setStatus("connecting", "")
		err := cs.ingestFromURL(client, url)
		if cs.ctx.Err() != nil {
			return
		}

		errMsg := "disconnected"
		if err != nil {
			errMsg = err.Error()
		}
		cs.setStatus("offline", errMsg)
		log.Printf("[Stream %s] Ingestão falhou (%v). Reconectando em %v...", cs.deviceID, errMsg, backoff)

		select {
		case <-cs.ctx.Done():
			return
		case <-time.After(backoff):
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (cs *CameraStream) ingestFromURL(client *http.Client, url string) error {
	req, err := http.NewRequestWithContext(cs.ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("câmera respondeu com HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	boundary := extractBoundary(contentType)

	cs.setStatus("streaming", "")
	log.Printf("[Stream %s] Ingestão conectada com sucesso a %s (boundary: %s)", cs.deviceID, url, boundary)

	reader := NewFrameReader(resp.Body, boundary)

	for {
		if cs.ctx.Err() != nil {
			return cs.ctx.Err()
		}

		frame, err := reader.NextFrame()
		if err != nil {
			return err
		}

		atomic.AddUint64(&cs.frameCount, 1)
		cs.updateFPS()
		cs.broadcaster.Broadcast(frame)
	}
}

func (cs *CameraStream) updateFPS() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(cs.lastFPSTime).Seconds()
	if elapsed >= 1.0 {
		total := atomic.LoadUint64(&cs.frameCount)
		cs.currentFPS = float64(total-cs.lastFPSCount) / elapsed
		cs.lastFPSTime = now
		cs.lastFPSCount = total
	}
}

func extractBoundary(contentType string) string {
	if contentType == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err == nil {
		if b, ok := params["boundary"]; ok {
			return b
		}
	}

	if idx := strings.Index(contentType, "boundary="); idx != -1 {
		b := contentType[idx+len("boundary="):]
		if semi := strings.Index(b, ";"); semi != -1 {
			b = b[:semi]
		}
		return strings.Trim(strings.TrimSpace(b), "\"")
	}
	return ""
}

func (cs *CameraStream) ServeLiveHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=--frame")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Connection", "close")

	ch, unsubscribe := cs.broadcaster.Subscribe()
	defer unsubscribe()

	if cs.broadcaster.LastFrame() == nil {
		placeholder := GeneratePlaceholderFrame(fmt.Sprintf("Aguardando câmera %s...", cs.deviceID))
		writeMultipartFrame(w, placeholder)
		flusher.Flush()
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-cs.ctx.Done():
			return
		case frame, ok := <-ch:
			if !ok {
				return
			}
			if err := writeMultipartFrame(w, frame); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeMultipartFrame(w http.ResponseWriter, frame []byte) error {
	header := fmt.Sprintf("--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(frame))
	if _, err := w.Write([]byte(header)); err != nil {
		return err
	}
	if _, err := w.Write(frame); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\r\n")); err != nil {
		return err
	}
	return nil
}

func GeneratePlaceholderFrame(text string) []byte {
	width, height := 640, 480
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	bg := color.RGBA{R: 20, G: 25, B: 30, A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	centerRect := image.Rect(width/4, height/3, 3*width/4, 2*height/3)
	cardBg := color.RGBA{R: 35, G: 40, B: 48, A: 255}
	draw.Draw(img, centerRect, &image.Uniform{C: cardBg}, image.Point{}, draw.Src)

	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 75})
	return buf.Bytes()
}
