package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"synthezia/internal/database"
	"synthezia/internal/models"
	"synthezia/internal/transcription"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateLiveSessionRequest models the payload to bootstrap a live transcription session.
type CreateLiveSessionRequest struct {
	Title      *string                `json:"title"`
	Parameters *models.WhisperXParams `json:"parameters"`
}

// CreateLiveSession spins up a new live transcription session and returns its metadata.
func (h *Handler) CreateLiveSession(c *gin.Context) {
	if h.liveTranscription == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Live transcription is not enabled"})
		return
	}

	var req CreateLiveSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session, err := h.liveTranscription.CreateSession(c.Request.Context(), transcription.CreateLiveSessionInput{
		Title:      req.Title,
		Parameters: req.Parameters,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, session)
}

// GetLiveSession returns the current state of a live session.
func (h *Handler) GetLiveSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	session, err := h.liveTranscription.GetSession(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, session)
}

// UploadLiveChunk ingests a single audio chunk for live processing.
func (h *Handler) UploadLiveChunk(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data"})
		return
	}

	file, header, err := c.Request.FormFile("chunk")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chunk file is required"})
		return
	}
	defer file.Close()

	sequenceStr := c.PostForm("sequence")
	seq, err := strconv.Atoi(sequenceStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sequence"})
		return
	}

	startOffset, err := strconv.ParseFloat(c.PostForm("start_offset"), 64)
	if err != nil {
		startOffset = 0
	}
	endOffset, err := strconv.ParseFloat(c.PostForm("end_offset"), 64)
	if err != nil {
		endOffset = 0
	}

	sessionID := c.Param("session_id")
	result, err := h.liveTranscription.AppendChunk(c.Request.Context(), sessionID, transcription.ChunkMetadata{
		Sequence:    seq,
		StartOffset: startOffset,
		EndOffset:   endOffset,
		ContentType: header.Header.Get("Content-Type"),
		Filename:    header.Filename,
	}, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"chunk":      result.Chunk,
		"transcript": result.Transcript,
	})
}

// StreamLiveSession streams JSON lines with realtime transcript updates.
func (h *Handler) StreamLiveSession(c *gin.Context) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}

	sessionID := c.Param("session_id")
	snapshots, stream, cancel, err := h.liveTranscription.Subscribe(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	defer cancel()

	c.Header("Content-Type", "text/plain")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	writer := c.Writer

	writePayload := func(payload transcription.LiveTranscriptPayload) bool {
		data, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		if _, err := writer.Write(append(data, '\n')); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	for _, payload := range snapshots {
		if !writePayload(payload) {
			return
		}
	}

	for {
		select {
		case payload, ok := <-stream:
			if !ok {
				return
			}
			if !writePayload(payload) {
				return
			}
		case <-c.Request.Context().Done():
			return
		}
	}
}

// FinalizeLiveSession closes the live session, merges audio, and enqueues a traditional job.
func (h *Handler) FinalizeLiveSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	finalizeResult, err := h.liveTranscription.FinalizeSession(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session := finalizeResult.Session
	jobID := uuid.New().String()
	title := session.Title
	job := &models.TranscriptionJob{
		ID:         jobID,
		AudioPath:  finalizeResult.MergedAudio,
		Status:     models.StatusPending,
		Parameters: session.Parameters,
	}
	if title != nil {
		job.Title = title
	}

	if err := database.DB.Create(job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create final job"})
		return
	}

	if err := h.taskQueue.EnqueueJob(jobID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue job"})
		return
	}

	now := time.Now()
	session.Status = models.LiveStatusCompleted
	session.FinalJobID = &jobID
	session.CompletedAt = &now
	if err := database.DB.Save(session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.liveTranscription.EmitStatus(session)

	c.JSON(http.StatusOK, gin.H{
		"session": session,
		"job":     job,
	})
}

// CancelLiveSession aborts a live session.
func (h *Handler) CancelLiveSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	session, err := h.liveTranscription.CancelSession(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, session)
}
