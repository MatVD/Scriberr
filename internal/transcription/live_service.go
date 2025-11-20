package transcription

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"synthezia/internal/config"
	"synthezia/internal/database"
	"synthezia/internal/models"
	"synthezia/internal/transcription/interfaces"
	"synthezia/pkg/logger"
)

// LiveTranscriptionService coordinates progressive/live transcription sessions.
type LiveTranscriptionService struct {
	cfg      *config.Config
	unified  *UnifiedTranscriptionService
	baseDir  string
	locks    sync.Map // map[string]*sync.Mutex
	sessions sync.Map // map[string]*sessionBroadcaster
}

// LiveTranscriptPayload is streamed to clients to communicate updates.
type LiveTranscriptPayload struct {
	Type          string                   `json:"type"`
	SessionID     string                   `json:"session_id"`
	SessionStatus models.LiveSessionStatus `json:"session_status"`
	Title         *string                  `json:"title,omitempty"`
	Chunk         *LiveChunkPayload        `json:"chunk,omitempty"`
	Chunks        []LiveChunkPayload       `json:"chunks,omitempty"`
	Accumulated   *string                  `json:"accumulated_text,omitempty"`
	FinalJobID    *string                  `json:"final_job_id,omitempty"`
	Timestamp     time.Time                `json:"timestamp"`
}

// LiveChunkPayload captures a single chunk update for streaming clients.
type LiveChunkPayload struct {
	Sequence    int             `json:"sequence"`
	StartOffset float64         `json:"start_offset"`
	EndOffset   float64         `json:"end_offset"`
	Text        string          `json:"text"`
	Segments    []StreamSegment `json:"segments,omitempty"`
}

// StreamSegment mirrors TranscriptSegment but keeps payloads focused.
type StreamSegment struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Text    string  `json:"text"`
	Speaker *string `json:"speaker,omitempty"`
}

// CreateLiveSessionInput represents the inputs necessary to bootstrap a live session.
type CreateLiveSessionInput struct {
	Title      *string
	Parameters *models.WhisperXParams
}

// ChunkMetadata describes an incoming chunk.
type ChunkMetadata struct {
	Sequence    int
	StartOffset float64
	EndOffset   float64
	ContentType string
	Filename    string
}

// LiveChunkResult wraps the processed chunk output.
type LiveChunkResult struct {
	Chunk      *models.LiveTranscriptionChunk
	Transcript *interfaces.TranscriptResult
}

// LiveFinalizeResult exposes data after merging audio for final processing.
type LiveFinalizeResult struct {
	Session     *models.LiveTranscriptionSession
	MergedAudio string
}

// sessionBroadcaster fans out live updates for a session.
type sessionBroadcaster struct {
	mu          sync.RWMutex
	subscribers map[int]chan LiveTranscriptPayload
	nextID      int
}

func newSessionBroadcaster() *sessionBroadcaster {
	return &sessionBroadcaster{subscribers: make(map[int]chan LiveTranscriptPayload)}
}

func (b *sessionBroadcaster) add(ch chan LiveTranscriptPayload) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextID
	b.nextID++
	b.subscribers[id] = ch
	return id
}

func (b *sessionBroadcaster) remove(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscribers, id)
}

func (b *sessionBroadcaster) broadcast(payload LiveTranscriptPayload) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- payload:
		default:
			// Drop message if subscriber is slow to avoid blocking hot path
		}
	}
}

// NewLiveTranscriptionService builds a live transcription coordinator on top of the unified service.
func NewLiveTranscriptionService(cfg *config.Config, unified *UnifiedTranscriptionService) (*LiveTranscriptionService, error) {
	baseDir := filepath.Join(cfg.UploadDir, "live_sessions")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create live session directory: %w", err)
	}

	return &LiveTranscriptionService{
		cfg:     cfg,
		unified: unified,
		baseDir: baseDir,
	}, nil
}

// CreateSession persists metadata for a new live transcription session.
func (s *LiveTranscriptionService) CreateSession(ctx context.Context, input CreateLiveSessionInput) (*models.LiveTranscriptionSession, error) {
	params := defaultLiveParameters()
	if input.Parameters != nil {
		params = *input.Parameters
	}

	session := &models.LiveTranscriptionSession{
		Title:      input.Title,
		Parameters: params,
		Status:     models.LiveStatusActive,
	}

	if err := database.DB.WithContext(ctx).Create(session).Error; err != nil {
		return nil, fmt.Errorf("failed to create live session: %w", err)
	}

	// Ensure on-disk structure exists ahead of time
	if err := os.MkdirAll(s.sessionDir(session.ID), 0755); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	s.EmitStatus(session)
	return session, nil
}

// GetSession fetches a session by ID.
func (s *LiveTranscriptionService) GetSession(ctx context.Context, sessionID string) (*models.LiveTranscriptionSession, error) {
	var session models.LiveTranscriptionSession
	if err := database.DB.WithContext(ctx).Where("id = ?", sessionID).First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

// AppendChunk stores, normalizes, and transcribes a chunk for the given session.
func (s *LiveTranscriptionService) AppendChunk(ctx context.Context, sessionID string, meta ChunkMetadata, reader io.Reader) (*LiveChunkResult, error) {
	lock := s.getSessionLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	var session models.LiveTranscriptionSession
	if err := database.DB.WithContext(ctx).Where("id = ?", sessionID).First(&session).Error; err != nil {
		return nil, err
	}

	if session.Status != models.LiveStatusActive {
		return nil, fmt.Errorf("session %s is no longer active", sessionID)
	}

	if meta.Sequence <= session.LastSequence {
		return nil, fmt.Errorf("sequence %d already processed", meta.Sequence)
	}

	normalizedPath, err := s.persistChunk(sessionID, meta.Sequence, meta.Filename, reader)
	if err != nil {
		return nil, err
	}

	transcript, err := s.unified.TranscribeFile(ctx, normalizedPath, session.Parameters)
	if err != nil {
		return nil, fmt.Errorf("chunk transcription failed: %w", err)
	}

	payload := LiveChunkPayload{
		Sequence:    meta.Sequence,
		StartOffset: meta.StartOffset,
		EndOffset:   meta.EndOffset,
		Text:        "",
	}
	if transcript != nil {
		payload.Text = transcript.Text
		if len(transcript.Segments) > 0 {
			payload.Segments = make([]StreamSegment, len(transcript.Segments))
			for i, seg := range transcript.Segments {
				payload.Segments[i] = StreamSegment{
					Start:   seg.Start,
					End:     seg.End,
					Text:    seg.Text,
					Speaker: seg.Speaker,
				}
			}
		}
	}

	var transcriptJSON *string
	if transcript != nil {
		data, err := json.Marshal(transcript)
		if err == nil {
			str := string(data)
			transcriptJSON = &str
		}
	}

	chunk := &models.LiveTranscriptionChunk{
		SessionID:      sessionID,
		Sequence:       meta.Sequence,
		StartOffset:    meta.StartOffset,
		EndOffset:      meta.EndOffset,
		AudioPath:      normalizedPath,
		TranscriptJSON: transcriptJSON,
	}
	if err := database.DB.WithContext(ctx).Create(chunk).Error; err != nil {
		return nil, fmt.Errorf("failed to persist chunk: %w", err)
	}

	session.ChunkCount++
	session.LastSequence = meta.Sequence
	if transcript != nil {
		accumulated := transcript.Text
		if session.AccumulatedTranscript != nil && *session.AccumulatedTranscript != "" {
			accumulated = *session.AccumulatedTranscript + "\n" + transcript.Text
		}
		session.AccumulatedTranscript = &accumulated
	}
	session.UpdatedAt = time.Now()

	if err := database.DB.WithContext(ctx).Save(&session).Error; err != nil {
		return nil, fmt.Errorf("failed to update session metadata: %w", err)
	}

	s.EmitChunk(&session, payload)

	return &LiveChunkResult{Chunk: chunk, Transcript: transcript}, nil
}

// FinalizeSession concatenates chunk audio to a single artifact ready for the offline pipeline.
func (s *LiveTranscriptionService) FinalizeSession(ctx context.Context, sessionID string) (*LiveFinalizeResult, error) {
	lock := s.getSessionLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	var session models.LiveTranscriptionSession
	if err := database.DB.WithContext(ctx).Where("id = ?", sessionID).First(&session).Error; err != nil {
		return nil, err
	}

	if session.Status != models.LiveStatusActive {
		return nil, fmt.Errorf("session %s cannot be finalized in status %s", session.ID, session.Status)
	}

	var chunks []models.LiveTranscriptionChunk
	if err := database.DB.WithContext(ctx).Where("session_id = ?", sessionID).Order("sequence ASC").Find(&chunks).Error; err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("session %s has no chunks", sessionID)
	}

	mergedPath := filepath.Join(s.sessionDir(sessionID), "merged.wav")
	if err := s.concatChunks(chunks, mergedPath); err != nil {
		return nil, fmt.Errorf("failed to merge audio: %w", err)
	}

	session.Status = models.LiveStatusFinalizing
	session.OutputAudioPath = &mergedPath
	session.UpdatedAt = time.Now()

	if err := database.DB.WithContext(ctx).Save(&session).Error; err != nil {
		return nil, err
	}

	s.EmitStatus(&session)

	return &LiveFinalizeResult{Session: &session, MergedAudio: mergedPath}, nil
}

// CancelSession marks a live session as cancelled and notifies listeners.
func (s *LiveTranscriptionService) CancelSession(ctx context.Context, sessionID string) (*models.LiveTranscriptionSession, error) {
	lock := s.getSessionLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	var session models.LiveTranscriptionSession
	if err := database.DB.WithContext(ctx).Where("id = ?", sessionID).First(&session).Error; err != nil {
		return nil, err
	}

	if session.Status == models.LiveStatusCancelled {
		return &session, nil
	}

	session.Status = models.LiveStatusCancelled
	now := time.Now()
	session.CompletedAt = &now

	if err := database.DB.WithContext(ctx).Save(&session).Error; err != nil {
		return nil, err
	}

	s.EmitStatus(&session)
	return &session, nil
}

// Subscribe wires a caller into live updates for the session.
func (s *LiveTranscriptionService) Subscribe(ctx context.Context, sessionID string) ([]LiveTranscriptPayload, <-chan LiveTranscriptPayload, func(), error) {
	var session models.LiveTranscriptionSession
	if err := database.DB.WithContext(ctx).Where("id = ?", sessionID).First(&session).Error; err != nil {
		return nil, nil, nil, err
	}

	var chunks []models.LiveTranscriptionChunk
	if err := database.DB.WithContext(ctx).Where("session_id = ?", sessionID).Order("sequence ASC").Find(&chunks).Error; err != nil {
		return nil, nil, nil, err
	}

	snapshotPayload := LiveTranscriptPayload{
		Type:          "snapshot",
		SessionID:     session.ID,
		SessionStatus: session.Status,
		Title:         session.Title,
		Timestamp:     time.Now(),
		Accumulated:   session.AccumulatedTranscript,
		FinalJobID:    session.FinalJobID,
	}

	if len(chunks) > 0 {
		payloads := make([]LiveChunkPayload, 0, len(chunks))
		for _, chunk := range chunks {
			payloads = append(payloads, LiveChunkPayload{
				Sequence:    chunk.Sequence,
				StartOffset: chunk.StartOffset,
				EndOffset:   chunk.EndOffset,
				Text:        chunkText(chunk.TranscriptJSON),
			})
		}
		snapshotPayload.Chunks = payloads
	}

	broadcaster := s.getBroadcaster(sessionID)
	updateChan := make(chan LiveTranscriptPayload, 16)
	subscriberID := broadcaster.add(updateChan)

	cancel := func() {
		broadcaster.remove(subscriberID)
		close(updateChan)
	}

	return []LiveTranscriptPayload{snapshotPayload}, updateChan, cancel, nil
}

// EmitChunk broadcasts a chunk payload to subscribers.
func (s *LiveTranscriptionService) EmitChunk(session *models.LiveTranscriptionSession, payload LiveChunkPayload) {
	broadcaster := s.getBroadcaster(session.ID)
	broadcaster.broadcast(LiveTranscriptPayload{
		Type:          "chunk",
		SessionID:     session.ID,
		SessionStatus: session.Status,
		Title:         session.Title,
		Chunk:         &payload,
		Accumulated:   session.AccumulatedTranscript,
		FinalJobID:    session.FinalJobID,
		Timestamp:     time.Now(),
	})
}

// EmitStatus broadcasts the latest session status.
func (s *LiveTranscriptionService) EmitStatus(session *models.LiveTranscriptionSession) {
	broadcaster := s.getBroadcaster(session.ID)
	broadcaster.broadcast(LiveTranscriptPayload{
		Type:          "status",
		SessionID:     session.ID,
		SessionStatus: session.Status,
		Title:         session.Title,
		Accumulated:   session.AccumulatedTranscript,
		FinalJobID:    session.FinalJobID,
		Timestamp:     time.Now(),
	})
}

func (s *LiveTranscriptionService) sessionDir(sessionID string) string {
	return filepath.Join(s.baseDir, sessionID)
}

func (s *LiveTranscriptionService) persistChunk(sessionID string, sequence int, filename string, reader io.Reader) (string, error) {
	sessionDir := s.sessionDir(sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return "", err
	}

	baseName := fmt.Sprintf("chunk_%05d", sequence)
	if filename == "" {
		filename = baseName + ".webm"
	}

	rawPath := filepath.Join(sessionDir, baseName+"_raw"+filepath.Ext(filename))
	rawFile, err := os.Create(rawPath)
	if err != nil {
		return "", err
	}

	n, err := io.Copy(rawFile, reader)
	if err != nil {
		rawFile.Close()
		return "", err
	}

	if err := rawFile.Close(); err != nil {
		return "", err
	}

	// Validate file size (must be > 1KB for valid audio)
	if n < 1024 {
		return "", fmt.Errorf("chunk too small (%d bytes), likely corrupted upload", n)
	}

	normalizedPath := filepath.Join(sessionDir, baseName+".wav")
	if err := s.convertToWav(rawPath, normalizedPath); err != nil {
		logger.Error("Failed to convert live chunk to wav",
			"session_id", sessionID,
			"sequence", sequence,
			"input", rawPath,
			"output", normalizedPath,
			"error", err)
		return "", fmt.Errorf("failed to convert chunk to wav: %w", err)
	}
	return normalizedPath, nil
}

func (s *LiveTranscriptionService) convertToWav(inputPath, outputPath string) error {
	// First, check if input file exists and has reasonable size
	info, err := os.Stat(inputPath)
	if err != nil {
		return fmt.Errorf("input file check failed: %w", err)
	}
	if info.Size() < 1024 {
		return fmt.Errorf("input file too small (%d bytes), likely corrupted", info.Size())
	}

	// With stop/start cycling, each chunk should be a complete WebM container
	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", inputPath,
		"-ar", "16000",
		"-ac", "1",
		"-c:a", "pcm_s16le",
		outputPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg convert failed: %v (%s)", err, string(out))
	}
	return nil
}

func (s *LiveTranscriptionService) concatChunks(chunks []models.LiveTranscriptionChunk, outputPath string) error {
	listPath := outputPath + ".txt"
	listFile, err := os.Create(listPath)
	if err != nil {
		return err
	}
	defer listFile.Close()

	for _, chunk := range chunks {
		chunkPath := chunk.AudioPath
		if !filepath.IsAbs(chunkPath) {
			absPath, err := filepath.Abs(chunkPath)
			if err != nil {
				return err
			}
			chunkPath = absPath
		}
		if _, err := fmt.Fprintf(listFile, "file '%s'\n", chunkPath); err != nil {
			return err
		}
	}

	cmd := exec.Command("ffmpeg",
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c", "copy",
		outputPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg concat failed: %v (%s)", err, string(out))
	}
	return nil
}

func (s *LiveTranscriptionService) getSessionLock(sessionID string) *sync.Mutex {
	if val, ok := s.locks.Load(sessionID); ok {
		return val.(*sync.Mutex)
	}
	mutex := &sync.Mutex{}
	actual, _ := s.locks.LoadOrStore(sessionID, mutex)
	return actual.(*sync.Mutex)
}

func (s *LiveTranscriptionService) getBroadcaster(sessionID string) *sessionBroadcaster {
	if val, ok := s.sessions.Load(sessionID); ok {
		return val.(*sessionBroadcaster)
	}
	broadcaster := newSessionBroadcaster()
	actual, _ := s.sessions.LoadOrStore(sessionID, broadcaster)
	return actual.(*sessionBroadcaster)
}

func defaultLiveParameters() models.WhisperXParams {
	return models.WhisperXParams{
		ModelFamily: "whisper",
		Model:       "small",
		Device:      "auto",
		BatchSize:   8,
		ComputeType: "float32",
		ChunkSize:   30,
		VadMethod:   "pyannote",
		VadOnset:    0.5,
		VadOffset:   0.363,
	}
}

func chunkText(blob *string) string {
	if blob == nil || *blob == "" {
		return ""
	}
	var result interfaces.TranscriptResult
	if err := json.Unmarshal([]byte(*blob), &result); err != nil {
		return ""
	}
	return result.Text
}

// SegmentInfo is an alias to keep payloads light without exposing every struct detail.
// We embed it here to avoid circular deps when marshalling.
