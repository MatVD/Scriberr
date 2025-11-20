package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"synthezia/internal/models"
	"synthezia/internal/processing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ProcessingTestSuite struct {
	suite.Suite
	helper    *TestHelper
	processor *processing.MultiTrackProcessor
	testDir   string
}

func (suite *ProcessingTestSuite) SetupSuite() {
	suite.helper = NewTestHelper(suite.T(), "processing_test.db")
	suite.processor = processing.NewMultiTrackProcessor()
	suite.testDir = "test_multitrack_data"
	os.MkdirAll(suite.testDir, 0755)
}

func (suite *ProcessingTestSuite) TearDownSuite() {
	os.RemoveAll(suite.testDir)
	suite.helper.Cleanup()
}

// Test MultiTrackProcessor creation
func (suite *ProcessingTestSuite) TestNewMultiTrackProcessor() {
	processor := processing.NewMultiTrackProcessor()
	assert.NotNil(suite.T(), processor)
}

// Test ProcessMultiTrackJob with non-existent job
func (suite *ProcessingTestSuite) TestProcessMultiTrackJobNotFound() {
	ctx := context.Background()
	err := suite.processor.ProcessMultiTrackJob(ctx, "nonexistent-job-id")
	
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "failed to find job")
}

// Test ProcessMultiTrackJob with non-multitrack job
func (suite *ProcessingTestSuite) TestProcessMultiTrackJobNotMultiTrack() {
	ctx := context.Background()

	// Create a regular (non-multitrack) job
	job := suite.helper.CreateTestTranscriptionJob(suite.T(), "Regular Job")
	
	err := suite.processor.ProcessMultiTrackJob(ctx, job.ID)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "not a multi-track job")
}

// Test ProcessMultiTrackJob with invalid AUP file
func (suite *ProcessingTestSuite) TestProcessMultiTrackJobInvalidAup() {
	ctx := context.Background()

	// Create multitrack folder
	multiTrackFolder := filepath.Join(suite.testDir, "invalid_aup_test")
	os.MkdirAll(multiTrackFolder, 0755)

	// Create invalid AUP file
	aupPath := filepath.Join(multiTrackFolder, "project.aup")
	os.WriteFile(aupPath, []byte("invalid xml content"), 0644)

	// Create multitrack job
	job := &models.TranscriptionJob{
		Title:             stringPtr("Invalid AUP Test"),
		Status:            models.StatusPending,
		AudioPath:         "",
		IsMultiTrack:      true,
		AupFilePath:       &aupPath,
		MultiTrackFolder:  &multiTrackFolder,
		MergeStatus:       "pending",
	}

	result := suite.helper.DB.Create(job)
	assert.NoError(suite.T(), result.Error)

	// Process should fail
	err := suite.processor.ProcessMultiTrackJob(ctx, job.ID)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "failed to parse AUP file")

	// Check that merge status was updated to failed
	var updatedJob models.TranscriptionJob
	suite.helper.DB.Where("id = ?", job.ID).First(&updatedJob)
	assert.Equal(suite.T(), "failed", updatedJob.MergeStatus)
	assert.NotNil(suite.T(), updatedJob.MergeError)
}

// Test ProcessMultiTrackJob with valid AUP but missing audio files
func (suite *ProcessingTestSuite) TestProcessMultiTrackJobMissingAudioFiles() {
	ctx := context.Background()

	// Create multitrack folder
	multiTrackFolder := filepath.Join(suite.testDir, "missing_files_test")
	os.MkdirAll(multiTrackFolder, 0755)

	// Create valid AUP file referencing non-existent audio files
	aupContent := `<?xml version="1.0" standalone="no" ?>
<project xmlns="http://audacity.sourceforge.net/xml/" audacityversion="2.4.2" rate="44100">
  <wavetrack name="Track 1" channel="0" linked="0" mute="0" solo="0" height="150" rate="44100" gain="1.0" pan="0.0">
    <waveclip offset="0.0">
      <import filename="nonexistent.wav" offset="0.0" channel="0"/>
    </waveclip>
  </wavetrack>
</project>`

	aupPath := filepath.Join(multiTrackFolder, "project.aup")
	os.WriteFile(aupPath, []byte(aupContent), 0644)

	// Create multitrack job
	job := &models.TranscriptionJob{
		Title:             stringPtr("Missing Files Test"),
		Status:            models.StatusPending,
		AudioPath:         "",
		IsMultiTrack:      true,
		AupFilePath:       &aupPath,
		MultiTrackFolder:  &multiTrackFolder,
		MergeStatus:       "pending",
	}

	result := suite.helper.DB.Create(job)
	assert.NoError(suite.T(), result.Error)

	// Create matching MultiTrackFile record
	trackFile := &models.MultiTrackFile{
		TranscriptionJobID: job.ID,
		FileName:           "nonexistent",
		FilePath:           filepath.Join(multiTrackFolder, "nonexistent.wav"),
		TrackIndex:         0,
		Offset:             0.0,
		Gain:               1.0,
		Pan:                0.0,
		Mute:               false,
	}
	suite.helper.DB.Create(trackFile)

	// Process should fail during merge
	err := suite.processor.ProcessMultiTrackJob(ctx, job.ID)
	assert.Error(suite.T(), err)
	// Should fail at merge stage because audio files don't exist
}

// Test GetMergeStatus
func (suite *ProcessingTestSuite) TestGetMergeStatus() {
	// Create a job with merge status
	job := &models.TranscriptionJob{
		Title:       stringPtr("Merge Status Test"),
		Status:      models.StatusPending,
		AudioPath:   "test/path.mp3",
		MergeStatus: "processing",
	}

	result := suite.helper.DB.Create(job)
	assert.NoError(suite.T(), result.Error)

	// Get merge status
	status, errorMsg, err := suite.processor.GetMergeStatus(job.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "processing", status)
	assert.Nil(suite.T(), errorMsg)
}

// Test GetMergeStatus with error message
func (suite *ProcessingTestSuite) TestGetMergeStatusWithError() {
	// Create a job with failed merge status
	errorMessage := "Test error message"
	job := &models.TranscriptionJob{
		Title:       stringPtr("Failed Merge Test"),
		Status:      models.StatusPending,
		AudioPath:   "test/path.mp3",
		MergeStatus: "failed",
		MergeError:  &errorMessage,
	}

	result := suite.helper.DB.Create(job)
	assert.NoError(suite.T(), result.Error)

	// Get merge status
	status, errorMsg, err := suite.processor.GetMergeStatus(job.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "failed", status)
	assert.NotNil(suite.T(), errorMsg)
	assert.Equal(suite.T(), "Test error message", *errorMsg)
}

// Test GetMergeStatus with non-existent job
func (suite *ProcessingTestSuite) TestGetMergeStatusNotFound() {
	status, errorMsg, err := suite.processor.GetMergeStatus("nonexistent-job-id")
	
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "failed to get job")
	assert.Empty(suite.T(), status)
	assert.Nil(suite.T(), errorMsg)
}

// Test complete workflow with valid multitrack job
func (suite *ProcessingTestSuite) TestProcessMultiTrackJobCompleteWorkflow() {
	ctx := context.Background()

	// Create multitrack folder
	multiTrackFolder := filepath.Join(suite.testDir, "complete_workflow")
	os.MkdirAll(multiTrackFolder, 0755)

	// Create valid audio files (dummy data)
	audio1Path := filepath.Join(multiTrackFolder, "audio1.wav")
	audio2Path := filepath.Join(multiTrackFolder, "audio2.wav")
	os.WriteFile(audio1Path, []byte("dummy audio data 1"), 0644)
	os.WriteFile(audio2Path, []byte("dummy audio data 2"), 0644)

	// Create valid AUP file
	aupContent := `<?xml version="1.0" standalone="no" ?>
<project xmlns="http://audacity.sourceforge.net/xml/" audacityversion="2.4.2" rate="44100">
  <wavetrack name="Track 1" channel="0" linked="0" mute="0" solo="0" height="150" rate="44100" gain="1.0" pan="0.0">
    <waveclip offset="0.0">
      <import filename="audio1.wav" offset="0.0" channel="0"/>
    </waveclip>
  </wavetrack>
  <wavetrack name="Track 2" channel="1" linked="0" mute="0" solo="0" height="150" rate="44100" gain="0.8" pan="0.5">
    <waveclip offset="2.5">
      <import filename="audio2.wav" offset="2.5" channel="0"/>
    </waveclip>
  </wavetrack>
</project>`

	aupPath := filepath.Join(multiTrackFolder, "project.aup")
	os.WriteFile(aupPath, []byte(aupContent), 0644)

	// Create multitrack job
	job := &models.TranscriptionJob{
		Title:             stringPtr("Complete Workflow Test"),
		Status:            models.StatusPending,
		AudioPath:         "",
		IsMultiTrack:      true,
		AupFilePath:       &aupPath,
		MultiTrackFolder:  &multiTrackFolder,
		MergeStatus:       "pending",
	}

	result := suite.helper.DB.Create(job)
	assert.NoError(suite.T(), result.Error)

	// Create MultiTrackFile records
	trackFile1 := &models.MultiTrackFile{
		TranscriptionJobID: job.ID,
		FileName:           "audio1",
		FilePath:           audio1Path,
		TrackIndex:         0,
		Offset:             0.0,
		Gain:               1.0,
		Pan:                0.0,
		Mute:               false,
	}
	suite.helper.DB.Create(trackFile1)

	trackFile2 := &models.MultiTrackFile{
		TranscriptionJobID: job.ID,
		FileName:           "audio2",
		FilePath:           audio2Path,
		TrackIndex:         1,
		Offset:             0.0,
		Gain:               1.0,
		Pan:                0.0,
		Mute:               false,
	}
	suite.helper.DB.Create(trackFile2)

	// Process the job (will likely fail if ffmpeg not available, but tests the flow)
	err := suite.processor.ProcessMultiTrackJob(ctx, job.ID)
	
	// The test will likely fail at merge stage if ffmpeg is not available
	// but we can verify that the track offsets were updated correctly
	var updatedFiles []models.MultiTrackFile
	suite.helper.DB.Where("transcription_job_id = ?", job.ID).Find(&updatedFiles)
	
	if len(updatedFiles) == 2 {
		// Check that offsets were updated from AUP
		foundTrack1 := false
		foundTrack2 := false
		
		for _, file := range updatedFiles {
			if file.FileName == "audio1" {
				foundTrack1 = true
				assert.Equal(suite.T(), 0.0, file.Offset)
				assert.Equal(suite.T(), 1.0, file.Gain)
				assert.Equal(suite.T(), 0.0, file.Pan)
			}
			if file.FileName == "audio2" {
				foundTrack2 = true
				assert.Equal(suite.T(), 2.5, file.Offset)
				assert.Equal(suite.T(), 0.8, file.Gain)
				assert.Equal(suite.T(), 0.5, file.Pan)
			}
		}
		
		assert.True(suite.T(), foundTrack1, "Track 1 should be updated")
		assert.True(suite.T(), foundTrack2, "Track 2 should be updated")
	}
	
	// We expect an error if ffmpeg is not available
	// but that's okay for this test - we're testing the workflow
	_ = err
}

// Test updating track offsets with partial matches
func (suite *ProcessingTestSuite) TestUpdateTrackOffsetsPartialMatch() {
	ctx := context.Background()

	// Create multitrack folder
	multiTrackFolder := filepath.Join(suite.testDir, "partial_match")
	os.MkdirAll(multiTrackFolder, 0755)

	// Create AUP with only one track
	aupContent := `<?xml version="1.0" standalone="no" ?>
<project xmlns="http://audacity.sourceforge.net/xml/" audacityversion="2.4.2" rate="44100">
  <wavetrack name="Track 1" channel="0" linked="0" mute="0" solo="0" height="150" rate="44100" gain="1.0" pan="0.0">
    <waveclip offset="0.0">
      <import filename="audio1.wav" offset="0.0" channel="0"/>
    </waveclip>
  </wavetrack>
</project>`

	aupPath := filepath.Join(multiTrackFolder, "project.aup")
	os.WriteFile(aupPath, []byte(aupContent), 0644)

	// Create multitrack job
	job := &models.TranscriptionJob{
		Title:             stringPtr("Partial Match Test"),
		Status:            models.StatusPending,
		AudioPath:         "",
		IsMultiTrack:      true,
		AupFilePath:       &aupPath,
		MultiTrackFolder:  &multiTrackFolder,
		MergeStatus:       "pending",
	}

	result := suite.helper.DB.Create(job)
	assert.NoError(suite.T(), result.Error)

	// Create two track files, but only one exists in AUP
	trackFile1 := &models.MultiTrackFile{
		TranscriptionJobID: job.ID,
		FileName:           "audio1",
		FilePath:           filepath.Join(multiTrackFolder, "audio1.wav"),
		TrackIndex:         0,
	}
	suite.helper.DB.Create(trackFile1)

	trackFile2 := &models.MultiTrackFile{
		TranscriptionJobID: job.ID,
		FileName:           "audio2",
		FilePath:           filepath.Join(multiTrackFolder, "audio2.wav"),
		TrackIndex:         1,
	}
	suite.helper.DB.Create(trackFile2)

	// Process (will fail but should update offsets first)
	err := suite.processor.ProcessMultiTrackJob(ctx, job.ID)
	_ = err // Ignore error (merge will fail anyway)

	// Check that track1 was updated from AUP, track2 got default values
	var updatedFile1 models.MultiTrackFile
	suite.helper.DB.Where("id = ?", trackFile1.ID).First(&updatedFile1)
	assert.Equal(suite.T(), 0.0, updatedFile1.Offset)
	assert.Equal(suite.T(), 1.0, updatedFile1.Gain)

	var updatedFile2 models.MultiTrackFile
	suite.helper.DB.Where("id = ?", trackFile2.ID).First(&updatedFile2)
	// Should have default values since not in AUP
	assert.Equal(suite.T(), 0.0, updatedFile2.Offset)
	assert.Equal(suite.T(), 1.0, updatedFile2.Gain)
	assert.Equal(suite.T(), 0.0, updatedFile2.Pan)
}

func TestProcessingTestSuite(t *testing.T) {
	suite.Run(t, new(ProcessingTestSuite))
}
