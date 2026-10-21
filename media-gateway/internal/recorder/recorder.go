package recorder

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type CmdExecutor func(ctx context.Context, name string, args ...string) *exec.Cmd

func defaultCmdExecutor(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

type RecordingController struct {
	mu            sync.Mutex
	recordingsDir string
	tolerance     time.Duration
	sessions      map[string]*recordingSession
	store         *MetadataStore
	cmdExecutor   CmdExecutor
	closed        bool
}

func NewRecordingController(recordingsDir string, tolerance time.Duration, store *MetadataStore) *RecordingController {
	if tolerance <= 0 {
		tolerance = 30 * time.Second
	}
	_ = os.MkdirAll(recordingsDir, os.ModePerm)

	return &RecordingController{
		recordingsDir: recordingsDir,
		tolerance:     tolerance,
		sessions:      make(map[string]*recordingSession),
		store:         store,
		cmdExecutor:   defaultCmdExecutor,
	}
}

func (rc *RecordingController) SetCmdExecutor(executor CmdExecutor) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.cmdExecutor = executor
}

func (rc *RecordingController) TriggerEvent(deviceID, streamURL string) (string, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.closed {
		return "", fmt.Errorf("controlador de gravação encerrado")
	}

	if session, exists := rc.sessions[deviceID]; exists && session.isActive() {

		session.resetTimer(rc.tolerance)
		log.Printf("[Recorder %s] Movimento validado recebido durante gravação. Tolerância de %v reiniciada.",
			deviceID, rc.tolerance)
		return "recording_extended", nil
	}

	session, err := rc.startSessionLocked(deviceID, streamURL)
	if err != nil {
		return "", err
	}
	rc.sessions[deviceID] = session
	log.Printf("[Recorder %s] Nova gravação iniciada via evento PIR: arquivo=%s, tolerância=%v",
		deviceID, session.meta.FileName, rc.tolerance)
	return "recording_started", nil
}

func (rc *RecordingController) IsRecording(deviceID string) bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	session, exists := rc.sessions[deviceID]
	return exists && session.isActive()
}

func (rc *RecordingController) StopSession(deviceID string) error {
	rc.mu.Lock()
	session, exists := rc.sessions[deviceID]
	rc.mu.Unlock()

	if !exists || !session.isActive() {
		return nil
	}
	session.stop()
	return nil
}

func (rc *RecordingController) Shutdown() {
	rc.mu.Lock()
	rc.closed = true
	active := make([]*recordingSession, 0, len(rc.sessions))
	for _, s := range rc.sessions {
		if s.isActive() {
			active = append(active, s)
		}
	}
	rc.mu.Unlock()

	log.Printf("[Recorder] Encerramento gracioso solicitado para %d gravações ativas...", len(active))
	var wg sync.WaitGroup
	for _, s := range active {
		wg.Add(1)
		go func(sess *recordingSession) {
			defer wg.Done()
			sess.stop()
		}(s)
	}
	wg.Wait()
	log.Println("[Recorder] Todas as gravações foram finalizadas graciosamente.")
}

func (rc *RecordingController) startSessionLocked(deviceID, streamURL string) (*recordingSession, error) {
	now := time.Now()
	fileName := fmt.Sprintf("%s_%s.mp4", deviceID, now.Format("20060102_150405"))
	filePath := filepath.Join(rc.recordingsDir, fileName)

	meta := &RecordingMetadata{
		ID:        fileName,
		DeviceID:  deviceID,
		FilePath:  filePath,
		FileName:  fileName,
		StartedAt: now,
		Status:    "recording",
	}
	if rc.store != nil {
		rc.store.Save(meta)
	}

	ctx, cancel := context.WithCancel(context.Background())
	session := &recordingSession{
		deviceID:  deviceID,
		streamURL: streamURL,
		filePath:  filePath,
		meta:      meta,
		tolerance: rc.tolerance,
		resetChan: make(chan time.Duration, 10),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
		active:    true,
	}

	args := []string{
		"-y",
		"-i", streamURL,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		filePath,
	}

	cmd := rc.cmdExecutor(ctx, "ffmpeg", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("falha ao criar stdin pipe para ffmpeg: %w", err)
	}

	session.cmd = cmd
	session.stdin = stdin

	if err := cmd.Start(); err != nil {
		cancel()
		meta.Status = "failed"
		meta.ErrorReason = err.Error()
		return nil, fmt.Errorf("falha ao iniciar processo ffmpeg: %w", err)
	}

	go session.runLoop(rc)
	return session, nil
}

type recordingSession struct {
	mu        sync.Mutex
	deviceID  string
	streamURL string
	filePath  string
	meta      *RecordingMetadata
	tolerance time.Duration
	cmd       *exec.Cmd
	stdin     io.WriteCloser

	resetChan chan time.Duration
	stopChan  chan struct{}
	doneChan  chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	active    bool
}

func (s *recordingSession) isActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *recordingSession) resetTimer(d time.Duration) {
	select {
	case s.resetChan <- d:
	default:
	}
}

func (s *recordingSession) stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	s.cancel()
	<-s.doneChan
}

func (s *recordingSession) runLoop(rc *RecordingController) {
	s.mu.Lock()
	s.active = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.active = false
		s.mu.Unlock()
		close(s.doneChan)
		s.cancel()

		rc.mu.Lock()
		delete(rc.sessions, s.deviceID)
		rc.mu.Unlock()
	}()

	timer := time.NewTimer(s.tolerance)
	defer timer.Stop()

	stopped := false
	for !stopped {
		select {
		case d := <-s.resetChan:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(d)
		case <-s.stopChan:
			stopped = true
		case <-timer.C:
			log.Printf("[Recorder %s] Período de tolerância (%v) expirou sem novos eventos. Encerrando gravação...",
				s.deviceID, s.tolerance)
			stopped = true
		case <-s.ctx.Done():
			stopped = true
		}
	}

	s.gracefulStopFFmpeg()

	now := time.Now()
	s.meta.EndedAt = now
	s.meta.DurationSeconds = now.Sub(s.meta.StartedAt).Seconds()

	if info, err := os.Stat(s.filePath); err == nil {
		s.meta.SizeBytes = info.Size()
		s.meta.Status = "completed"
		log.Printf("[Recorder %s] Gravação finalizada com sucesso! Arquivo: %s (%.1fs, %d B)",
			s.deviceID, s.filePath, s.meta.DurationSeconds, s.meta.SizeBytes)
	} else {
		s.meta.Status = "failed"
		s.meta.ErrorReason = err.Error()
		log.Printf("[Recorder %s] Aviso: Arquivo de gravação não encontrado: %v", s.deviceID, err)
	}

	if rc.store != nil {
		rc.store.Save(s.meta)
		_ = rc.store.PersistManifest()
	}
}

func (s *recordingSession) gracefulStopFFmpeg() {
	if s.stdin != nil {

		_, _ = s.stdin.Write([]byte("q\n"))
		_ = s.stdin.Close()
	}

	done := make(chan error, 1)
	go func() {
		done <- s.cmd.Wait()
	}()

	select {
	case <-done:

		return
	case <-time.After(3 * time.Second):

		if s.cmd.Process != nil {
			_ = s.cmd.Process.Signal(syscall.SIGINT)
		}
	}

	select {
	case <-done:
		return
	case <-time.After(2 * time.Second):

		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
	}
}
