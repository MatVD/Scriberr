package tests

import (
	"synthezia/internal/config"
	"synthezia/internal/transcription"
)

// NewMockUnifiedJobProcessor creates a mock unified job processor
func NewMockUnifiedJobProcessor() *transcription.UnifiedJobProcessor {
	return transcription.NewUnifiedJobProcessor()
}

// NewMockLiveTranscriptionService creates a mock live transcription service
func NewMockLiveTranscriptionService(cfg *config.Config, processor *transcription.UnifiedJobProcessor) (*transcription.LiveTranscriptionService, error) {
	return transcription.NewLiveTranscriptionService(cfg, processor.GetUnifiedService())
}

// NewMockQuickTranscriptionService creates a mock quick transcription service
func NewMockQuickTranscriptionService(cfg *config.Config, processor *transcription.UnifiedJobProcessor) (*transcription.QuickTranscriptionService, error) {
	return transcription.NewQuickTranscriptionService(cfg, processor)
}
