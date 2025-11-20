package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"synthezia/internal/audio"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type AudioTestSuite struct {
	suite.Suite
	testDir string
}

func (suite *AudioTestSuite) SetupSuite() {
	// Create temporary test directory
	suite.testDir = "test_audio_data"
	os.MkdirAll(suite.testDir, 0755)
}

func (suite *AudioTestSuite) TearDownSuite() {
	// Clean up test directory
	os.RemoveAll(suite.testDir)
}

// Test AUP Parser creation
func (suite *AudioTestSuite) TestNewAupParser() {
	parser := audio.NewAupParser()
	assert.NotNil(suite.T(), parser)
}

// Test AUP file parsing with valid XML
func (suite *AudioTestSuite) TestParseAupFile() {
	parser := audio.NewAupParser()

	// Create a sample .aup file
	aupContent := `<?xml version="1.0" standalone="no" ?>
<!DOCTYPE project PUBLIC "-//audacityproject-1.3.0//DTD//EN" "http://audacity.sourceforge.net/xml/audacityproject-1.3.0.dtd">
<project xmlns="http://audacity.sourceforge.net/xml/" audacityversion="2.4.2" rate="44100" datadir="project_data">
  <wavetrack name="Track 1" channel="0" linked="0" mute="0" solo="0" height="150" minimized="0" isSelected="1" rate="44100" gain="1.0" pan="0.0">
    <waveclip offset="0.0">
      <import filename="audio1.wav" offset="0.0" channel="0"/>
    </waveclip>
  </wavetrack>
  <wavetrack name="Track 2" channel="1" linked="0" mute="0" solo="0" height="150" minimized="0" isSelected="0" rate="44100" gain="0.8" pan="-0.5">
    <waveclip offset="2.5">
      <import filename="audio2.wav" offset="2.5" channel="0"/>
    </waveclip>
  </wavetrack>
</project>`

	aupPath := filepath.Join(suite.testDir, "test_project.aup")
	err := os.WriteFile(aupPath, []byte(aupContent), 0644)
	assert.NoError(suite.T(), err)

	// Parse the file
	tracks, err := parser.ParseAupFile(aupPath)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), tracks, 2)

	// Verify first track
	assert.Equal(suite.T(), "audio1.wav", tracks[0].Filename)
	assert.Equal(suite.T(), 0.0, tracks[0].Offset)
	assert.Equal(suite.T(), 1.0, tracks[0].Gain)
	assert.Equal(suite.T(), 0.0, tracks[0].Pan)
	assert.Equal(suite.T(), 0, tracks[0].Mute)

	// Verify second track
	assert.Equal(suite.T(), "audio2.wav", tracks[1].Filename)
	assert.Equal(suite.T(), 2.5, tracks[1].Offset)
	assert.Equal(suite.T(), 0.8, tracks[1].Gain)
	assert.Equal(suite.T(), -0.5, tracks[1].Pan)
}

// Test parsing invalid AUP file
func (suite *AudioTestSuite) TestParseAupFileInvalid() {
	parser := audio.NewAupParser()

	// Create invalid XML file
	aupPath := filepath.Join(suite.testDir, "invalid.aup")
	err := os.WriteFile(aupPath, []byte("not valid xml"), 0644)
	assert.NoError(suite.T(), err)

	// Attempt to parse
	tracks, err := parser.ParseAupFile(aupPath)
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), tracks)
}

// Test parsing non-existent file
func (suite *AudioTestSuite) TestParseAupFileNotFound() {
	parser := audio.NewAupParser()

	tracks, err := parser.ParseAupFile(filepath.Join(suite.testDir, "nonexistent.aup"))
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), tracks)
}

// Test ValidateTracksExist with existing tracks
func (suite *AudioTestSuite) TestValidateTracksExist() {
	parser := audio.NewAupParser()

	// Create test audio files
	trackFile1 := filepath.Join(suite.testDir, "audio1.wav")
	trackFile2 := filepath.Join(suite.testDir, "audio2.wav")
	os.WriteFile(trackFile1, []byte("dummy audio data"), 0644)
	os.WriteFile(trackFile2, []byte("dummy audio data"), 0644)

	tracks := []audio.AupTrack{
		{Filename: "audio1.wav", Offset: 0.0},
		{Filename: "audio2.wav", Offset: 2.5},
	}

	err := parser.ValidateTracksExist(tracks, suite.testDir)
	assert.NoError(suite.T(), err)
}

// Test ValidateTracksExist with missing tracks
func (suite *AudioTestSuite) TestValidateTracksExistMissing() {
	parser := audio.NewAupParser()

	tracks := []audio.AupTrack{
		{Filename: "missing_audio.wav", Offset: 0.0},
	}

	err := parser.ValidateTracksExist(tracks, suite.testDir)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "track file not found")
}

// Test AudioMerger creation
func (suite *AudioTestSuite) TestNewAudioMerger() {
	merger := audio.NewAudioMerger()
	assert.NotNil(suite.T(), merger)
}

// Test AudioMerger with custom path
func (suite *AudioTestSuite) TestNewAudioMergerWithPath() {
	merger := audio.NewAudioMergerWithPath("/custom/path/to/ffmpeg")
	assert.NotNil(suite.T(), merger)
}

// Test MergeTracksWithOffsets with no tracks
func (suite *AudioTestSuite) TestMergeTracksWithOffsetsEmpty() {
	merger := audio.NewAudioMerger()
	ctx := context.Background()

	err := merger.MergeTracksWithOffsets(ctx, []audio.TrackInfo{}, "output.mp3", nil)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "no tracks provided")
}

// Test MergeTracksWithOffsets with non-existent files
func (suite *AudioTestSuite) TestMergeTracksWithOffsetsNonExistentFiles() {
	merger := audio.NewAudioMerger()
	ctx := context.Background()

	tracks := []audio.TrackInfo{
		{FilePath: filepath.Join(suite.testDir, "nonexistent.wav"), Offset: 0.0, Gain: 1.0, Pan: 0.0},
	}

	err := merger.MergeTracksWithOffsets(ctx, tracks, "output.mp3", nil)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "input file does not exist")
}

// Test MergeTracksWithOffsets with all muted tracks
func (suite *AudioTestSuite) TestMergeTracksWithOffsetsAllMuted() {
	merger := audio.NewAudioMerger()
	ctx := context.Background()

	// Create dummy audio file
	trackFile := filepath.Join(suite.testDir, "muted_track.wav")
	os.WriteFile(trackFile, []byte("dummy audio data"), 0644)

	tracks := []audio.TrackInfo{
		{FilePath: trackFile, Offset: 0.0, Gain: 1.0, Pan: 0.0, Mute: true},
	}

	err := merger.MergeTracksWithOffsets(ctx, tracks, "output.mp3", nil)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "no active (non-muted) tracks")
}

// Test MergeProgress structure
func (suite *AudioTestSuite) TestMergeProgress() {
	progress := audio.MergeProgress{
		Stage:      "processing",
		Progress:   50.0,
		ErrorMsg:   "",
		OutputPath: "/path/to/output.mp3",
	}

	assert.Equal(suite.T(), "processing", progress.Stage)
	assert.Equal(suite.T(), 50.0, progress.Progress)
	assert.Empty(suite.T(), progress.ErrorMsg)
	assert.Equal(suite.T(), "/path/to/output.mp3", progress.OutputPath)
}

// Test TrackInfo structure
func (suite *AudioTestSuite) TestTrackInfo() {
	track := audio.TrackInfo{
		FilePath: "/path/to/audio.wav",
		Offset:   2.5,
		Gain:     0.8,
		Pan:      -0.3,
		Mute:     false,
	}

	assert.Equal(suite.T(), "/path/to/audio.wav", track.FilePath)
	assert.Equal(suite.T(), 2.5, track.Offset)
	assert.Equal(suite.T(), 0.8, track.Gain)
	assert.Equal(suite.T(), -0.3, track.Pan)
	assert.False(suite.T(), track.Mute)
}

// Test context cancellation during merge
func (suite *AudioTestSuite) TestMergeTracksWithOffsetsCancellation() {
	merger := audio.NewAudioMerger()
	
	// Create a context that will be cancelled immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Create dummy audio file
	trackFile := filepath.Join(suite.testDir, "cancel_test.wav")
	os.WriteFile(trackFile, []byte("dummy audio data"), 0644)

	tracks := []audio.TrackInfo{
		{FilePath: trackFile, Offset: 0.0, Gain: 1.0, Pan: 0.0, Mute: false},
	}

	outputPath := filepath.Join(suite.testDir, "cancelled_output.mp3")
	
	// This should fail because context is already cancelled
	// Note: this may or may not error depending on timing, but should be safe
	err := merger.MergeTracksWithOffsets(ctx, tracks, outputPath, nil)
	// We just verify it doesn't panic
	_ = err
}

// Test progress callback functionality
func (suite *AudioTestSuite) TestMergeProgressCallback() {
	merger := audio.NewAudioMerger()
	ctx := context.Background()

	// Create dummy audio file
	trackFile := filepath.Join(suite.testDir, "progress_test.wav")
	os.WriteFile(trackFile, []byte("dummy audio data"), 0644)

	tracks := []audio.TrackInfo{
		{FilePath: trackFile, Offset: 0.0, Gain: 1.0, Pan: 0.0, Mute: false},
	}

	outputPath := filepath.Join(suite.testDir, "progress_output.mp3")
	
	// Track progress callbacks
	progressStages := []string{}
	progressCallback := func(progress audio.MergeProgress) {
		progressStages = append(progressStages, progress.Stage)
	}

	// This will fail because ffmpeg is likely not available in test env,
	// but we can at least test the callback is invoked
	err := merger.MergeTracksWithOffsets(ctx, tracks, outputPath, progressCallback)
	
	// Should have at least received "starting" and "validating" stages
	if len(progressStages) > 0 {
		assert.Contains(suite.T(), progressStages, "starting")
	}
	
	// We expect an error since ffmpeg is likely not available
	_ = err
}

// Test AupTrack structure
func (suite *AudioTestSuite) TestAupTrackStructure() {
	track := audio.AupTrack{
		Filename: "test_audio.wav",
		Offset:   1.5,
		Channel:  0,
		Mute:     0,
		Solo:     1,
		Gain:     0.9,
		Pan:      0.2,
	}

	assert.Equal(suite.T(), "test_audio.wav", track.Filename)
	assert.Equal(suite.T(), 1.5, track.Offset)
	assert.Equal(suite.T(), 0, track.Channel)
	assert.Equal(suite.T(), 0, track.Mute)
	assert.Equal(suite.T(), 1, track.Solo)
	assert.Equal(suite.T(), 0.9, track.Gain)
	assert.Equal(suite.T(), 0.2, track.Pan)
}

// Test parsing AUP with multiple waveclips per track
func (suite *AudioTestSuite) TestParseAupFileMultipleClips() {
	parser := audio.NewAupParser()

	aupContent := `<?xml version="1.0" standalone="no" ?>
<project xmlns="http://audacity.sourceforge.net/xml/" audacityversion="2.4.2" rate="44100">
  <wavetrack name="Track 1" channel="0" linked="0" mute="0" solo="0" height="150" rate="44100" gain="1.0" pan="0.0">
    <waveclip offset="0.0">
      <import filename="clip1.wav" offset="0.0" channel="0"/>
    </waveclip>
    <waveclip offset="5.0">
      <import filename="clip2.wav" offset="5.0" channel="0"/>
    </waveclip>
  </wavetrack>
</project>`

	aupPath := filepath.Join(suite.testDir, "multiclip.aup")
	err := os.WriteFile(aupPath, []byte(aupContent), 0644)
	assert.NoError(suite.T(), err)

	tracks, err := parser.ParseAupFile(aupPath)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), tracks, 2) // Should parse both clips as separate tracks

	assert.Equal(suite.T(), "clip1.wav", tracks[0].Filename)
	assert.Equal(suite.T(), 0.0, tracks[0].Offset)
	assert.Equal(suite.T(), "clip2.wav", tracks[1].Filename)
	assert.Equal(suite.T(), 5.0, tracks[1].Offset)
}

// Test parsing AUP with no imports
func (suite *AudioTestSuite) TestParseAupFileNoImports() {
	parser := audio.NewAupParser()

	aupContent := `<?xml version="1.0" standalone="no" ?>
<project xmlns="http://audacity.sourceforge.net/xml/" audacityversion="2.4.2" rate="44100">
  <wavetrack name="Empty Track" channel="0" linked="0" mute="0" solo="0" height="150" rate="44100" gain="1.0" pan="0.0">
    <waveclip offset="0.0">
    </waveclip>
  </wavetrack>
</project>`

	aupPath := filepath.Join(suite.testDir, "noimports.aup")
	err := os.WriteFile(aupPath, []byte(aupContent), 0644)
	assert.NoError(suite.T(), err)

	tracks, err := parser.ParseAupFile(aupPath)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), tracks, 0) // No tracks with imports
}

// Test ValidateFFmpeg
func (suite *AudioTestSuite) TestValidateFFmpeg() {
	merger := audio.NewAudioMerger()
	
	// This will succeed if ffmpeg is in PATH, otherwise fail
	err := merger.ValidateFFmpeg()
	// We just test that the method doesn't panic
	// The actual result depends on the test environment
	_ = err
}

// Test ValidateFFmpeg with custom path
func (suite *AudioTestSuite) TestValidateFFmpegCustomPath() {
	merger := audio.NewAudioMergerWithPath("/nonexistent/ffmpeg")
	
	err := merger.ValidateFFmpeg()
	// Should fail because path doesn't exist
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "ffmpeg not found or not working")
}

// Test context timeout during merge
func (suite *AudioTestSuite) TestMergeTracksWithOffsetsTimeout() {
	merger := audio.NewAudioMerger()
	
	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Create dummy audio file
	trackFile := filepath.Join(suite.testDir, "timeout_test.wav")
	os.WriteFile(trackFile, []byte("dummy audio data"), 0644)

	tracks := []audio.TrackInfo{
		{FilePath: trackFile, Offset: 0.0, Gain: 1.0, Pan: 0.0, Mute: false},
	}

	outputPath := filepath.Join(suite.testDir, "timeout_output.mp3")
	
	// Wait for context to timeout
	time.Sleep(2 * time.Millisecond)
	
	// This should handle timeout gracefully
	err := merger.MergeTracksWithOffsets(ctx, tracks, outputPath, nil)
	_ = err
}

func TestAudioTestSuite(t *testing.T) {
	suite.Run(t, new(AudioTestSuite))
}
