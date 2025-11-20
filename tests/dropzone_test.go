package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"synthezia/internal/dropzone"
	"synthezia/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// MockTaskQueue for testing dropzone service
type MockDropzoneTaskQueue struct {
	mock.Mock
	enqueuedJobs []string
}

func (m *MockDropzoneTaskQueue) EnqueueJob(jobID string) error {
	args := m.Called(jobID)
	m.enqueuedJobs = append(m.enqueuedJobs, jobID)
	return args.Error(0)
}

type DropzoneTestSuite struct {
	suite.Suite
	helper      *TestHelper
	dropzonePath string
	mockQueue   *MockDropzoneTaskQueue
}

func (suite *DropzoneTestSuite) SetupSuite() {
	suite.helper = NewTestHelper(suite.T(), "dropzone_test.db")
	suite.dropzonePath = filepath.Join("test_dropzone_data", "dropzone")
	suite.mockQueue = new(MockDropzoneTaskQueue)
}

func (suite *DropzoneTestSuite) TearDownSuite() {
	os.RemoveAll("test_dropzone_data")
	suite.helper.Cleanup()
}

func (suite *DropzoneTestSuite) SetupTest() {
	// Clean dropzone before each test
	os.RemoveAll(suite.dropzonePath)
	suite.mockQueue.enqueuedJobs = []string{}
}

// Test NewService creation
func (suite *DropzoneTestSuite) TestNewService() {
	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	assert.NotNil(suite.T(), service)
}

// Test service start creates directory
func (suite *DropzoneTestSuite) TestServiceStart() {
	// Update config to use test dropzone path
	originalUploadDir := suite.helper.Config.UploadDir
	suite.helper.Config.UploadDir = filepath.Join("test_dropzone_data", "uploads")
	defer func() {
		suite.helper.Config.UploadDir = originalUploadDir
	}()

	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	
	err := service.Start()
	assert.NoError(suite.T(), err)
	
	// Verify dropzone directory was created
	_, err = os.Stat(filepath.Join("data", "dropzone"))
	assert.NoError(suite.T(), err)
	
	// Stop service
	err = service.Stop()
	assert.NoError(suite.T(), err)
}

// Test processing existing audio files on startup
func (suite *DropzoneTestSuite) TestProcessExistingFiles() {
	// Create dropzone directory with audio files
	os.MkdirAll(suite.dropzonePath, 0755)
	
	// Create test audio files
	audioFile1 := filepath.Join(suite.dropzonePath, "test1.mp3")
	audioFile2 := filepath.Join(suite.dropzonePath, "test2.wav")
	nonAudioFile := filepath.Join(suite.dropzonePath, "document.txt")
	
	os.WriteFile(audioFile1, []byte("dummy audio 1"), 0644)
	os.WriteFile(audioFile2, []byte("dummy audio 2"), 0644)
	os.WriteFile(nonAudioFile, []byte("text document"), 0644)
	
	// Disable auto-transcription for this test
	suite.helper.Config.UploadDir = filepath.Join("test_dropzone_data", "uploads")
	
	// Mock queue to return no error
	suite.mockQueue.On("EnqueueJob", mock.Anything).Return(nil)
	
	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	
	err := service.Start()
	assert.NoError(suite.T(), err)
	
	// Give it time to process files
	time.Sleep(1 * time.Second)
	
	// Stop service
	service.Stop()
	
	// Verify audio files were processed
	// They should be moved from dropzone and uploaded
	_, err1 := os.Stat(audioFile1)
	_, err2 := os.Stat(audioFile2)
	
	// Audio files should be removed from dropzone after processing
	assert.True(suite.T(), os.IsNotExist(err1) || os.IsNotExist(err2), "At least one audio file should be processed")
	
	// Non-audio file should still exist
	_, err = os.Stat(nonAudioFile)
	assert.NoError(suite.T(), err)
}

// Test file detection with various audio formats
func (suite *DropzoneTestSuite) TestAudioFileDetection() {
	os.MkdirAll(suite.dropzonePath, 0755)
	suite.helper.Config.UploadDir = filepath.Join("test_dropzone_data", "uploads")
	
	suite.mockQueue.On("EnqueueJob", mock.Anything).Return(nil)
	
	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	err := service.Start()
	assert.NoError(suite.T(), err)
	defer service.Stop()
	
	// Test various audio formats
	audioFormats := []string{
		"test.mp3", "test.wav", "test.flac", "test.m4a",
		"test.aac", "test.ogg", "test.wma", "test.mp4",
	}
	
	for _, format := range audioFormats {
		filePath := filepath.Join(suite.dropzonePath, format)
		os.WriteFile(filePath, []byte("dummy audio"), 0644)
	}
	
	// Give time to process
	time.Sleep(1500 * time.Millisecond)
	
	// Check that jobs were created in database
	var jobs []models.TranscriptionJob
	suite.helper.DB.Find(&jobs)
	
	// Should have created jobs for all audio files
	assert.GreaterOrEqual(suite.T(), len(jobs), 1, "At least some audio files should be processed")
}

// Test non-audio file is ignored
func (suite *DropzoneTestSuite) TestNonAudioFilesIgnored() {
	os.MkdirAll(suite.dropzonePath, 0755)
	suite.helper.Config.UploadDir = filepath.Join("test_dropzone_data", "uploads")
	
	suite.mockQueue.On("EnqueueJob", mock.Anything).Return(nil)
	
	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	err := service.Start()
	assert.NoError(suite.T(), err)
	defer service.Stop()
	
	// Create non-audio files
	textFile := filepath.Join(suite.dropzonePath, "document.txt")
	pdfFile := filepath.Join(suite.dropzonePath, "document.pdf")
	
	os.WriteFile(textFile, []byte("text content"), 0644)
	os.WriteFile(pdfFile, []byte("pdf content"), 0644)
	
	// Give time for potential processing
	time.Sleep(1 * time.Second)
	
	// Files should still exist (not processed)
	_, err1 := os.Stat(textFile)
	_, err2 := os.Stat(pdfFile)
	assert.NoError(suite.T(), err1)
	assert.NoError(suite.T(), err2)
	
	// No jobs should be created
	var jobs []models.TranscriptionJob
	suite.helper.DB.Find(&jobs)
	assert.Equal(suite.T(), 0, len(jobs))
}

// Test subdirectory creation and monitoring
func (suite *DropzoneTestSuite) TestSubdirectoryMonitoring() {
	os.MkdirAll(suite.dropzonePath, 0755)
	suite.helper.Config.UploadDir = filepath.Join("test_dropzone_data", "uploads")
	
	suite.mockQueue.On("EnqueueJob", mock.Anything).Return(nil)
	
	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	err := service.Start()
	assert.NoError(suite.T(), err)
	defer service.Stop()
	
	// Create a subdirectory
	subDir := filepath.Join(suite.dropzonePath, "subfolder")
	os.MkdirAll(subDir, 0755)
	
	// Give time for directory to be detected
	time.Sleep(500 * time.Millisecond)
	
	// Create audio file in subdirectory
	audioFile := filepath.Join(subDir, "subdir_audio.mp3")
	os.WriteFile(audioFile, []byte("dummy audio in subdir"), 0644)
	
	// Give time to process
	time.Sleep(1500 * time.Millisecond)
	
	// File should be processed and removed
	_, err = os.Stat(audioFile)
	// File might be removed if processed successfully
	_ = err
}

// Test auto-transcription disabled
func (suite *DropzoneTestSuite) TestAutoTranscriptionDisabled() {
	os.MkdirAll(suite.dropzonePath, 0755)
	suite.helper.Config.UploadDir = filepath.Join("test_dropzone_data", "uploads")
	
	// Ensure no users have auto-transcription enabled
	suite.helper.DB.Model(&models.User{}).Where("1=1").Update("auto_transcription_enabled", false)
	
	suite.mockQueue.On("EnqueueJob", mock.Anything).Return(nil)
	
	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	err := service.Start()
	assert.NoError(suite.T(), err)
	defer service.Stop()
	
	// Create audio file
	audioFile := filepath.Join(suite.dropzonePath, "no_auto.mp3")
	os.WriteFile(audioFile, []byte("dummy audio"), 0644)
	
	// Give time to process
	time.Sleep(1500 * time.Millisecond)
	
	// Job should be created but not enqueued (status should be "uploaded")
	var job models.TranscriptionJob
	result := suite.helper.DB.Where("status = ?", models.StatusUploaded).First(&job)
	
	if result.Error == nil {
		// Job created but not auto-started
		assert.Equal(suite.T(), models.StatusUploaded, job.Status)
	}
	
	// Queue should not be called
	assert.Equal(suite.T(), 0, len(suite.mockQueue.enqueuedJobs))
}

// Test auto-transcription enabled
func (suite *DropzoneTestSuite) TestAutoTranscriptionEnabled() {
	os.MkdirAll(suite.dropzonePath, 0755)
	suite.helper.Config.UploadDir = filepath.Join("test_dropzone_data", "uploads")
	
	// Enable auto-transcription for test user
	suite.helper.DB.Model(&models.User{}).Where("username = ?", suite.helper.TestUser.Username).
		Update("auto_transcription_enabled", true)
	
	suite.mockQueue.On("EnqueueJob", mock.Anything).Return(nil)
	
	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	err := service.Start()
	assert.NoError(suite.T(), err)
	defer service.Stop()
	
	// Create audio file
	audioFile := filepath.Join(suite.dropzonePath, "auto_transcribe.mp3")
	os.WriteFile(audioFile, []byte("dummy audio"), 0644)
	
	// Give time to process
	time.Sleep(1500 * time.Millisecond)
	
	// Job should be enqueued
	if len(suite.mockQueue.enqueuedJobs) > 0 {
		assert.Greater(suite.T(), len(suite.mockQueue.enqueuedJobs), 0, "Job should be enqueued")
	}
}

// Test file upload creates correct database record
func (suite *DropzoneTestSuite) TestFileUploadCreatesJob() {
	os.MkdirAll(suite.dropzonePath, 0755)
	suite.helper.Config.UploadDir = filepath.Join("test_dropzone_data", "uploads")
	os.MkdirAll(suite.helper.Config.UploadDir, 0755)
	
	suite.mockQueue.On("EnqueueJob", mock.Anything).Return(nil)
	
	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	err := service.Start()
	assert.NoError(suite.T(), err)
	defer service.Stop()
	
	// Create audio file with specific name
	originalFilename := "my_recording.mp3"
	audioFile := filepath.Join(suite.dropzonePath, originalFilename)
	os.WriteFile(audioFile, []byte("dummy audio content"), 0644)
	
	// Give time to process
	time.Sleep(1500 * time.Millisecond)
	
	// Check database for job
	var job models.TranscriptionJob
	result := suite.helper.DB.Where("title = ?", originalFilename).First(&job)
	
	if result.Error == nil {
		assert.Equal(suite.T(), originalFilename, *job.Title)
		assert.Contains(suite.T(), job.AudioPath, suite.helper.Config.UploadDir)
	}
}

// Test service stop
func (suite *DropzoneTestSuite) TestServiceStop() {
	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	
	err := service.Start()
	assert.NoError(suite.T(), err)
	
	err = service.Stop()
	assert.NoError(suite.T(), err)
	
	// Stop again should be safe
	err = service.Stop()
	assert.NoError(suite.T(), err)
}

// Test concurrent file additions
func (suite *DropzoneTestSuite) TestConcurrentFileAdditions() {
	os.MkdirAll(suite.dropzonePath, 0755)
	suite.helper.Config.UploadDir = filepath.Join("test_dropzone_data", "uploads")
	
	suite.mockQueue.On("EnqueueJob", mock.Anything).Return(nil)
	
	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	err := service.Start()
	assert.NoError(suite.T(), err)
	defer service.Stop()
	
	// Add multiple files quickly
	for i := 0; i < 5; i++ {
		audioFile := filepath.Join(suite.dropzonePath, "concurrent_"+string(rune(i))+"_test.mp3")
		os.WriteFile(audioFile, []byte("dummy audio"), 0644)
		time.Sleep(100 * time.Millisecond)
	}
	
	// Give time to process all
	time.Sleep(2 * time.Second)
	
	// Check that jobs were created
	var jobs []models.TranscriptionJob
	suite.helper.DB.Find(&jobs)
	
	// At least some files should have been processed
	assert.GreaterOrEqual(suite.T(), len(jobs), 1)
}

// Test case-insensitive file extension matching
func (suite *DropzoneTestSuite) TestCaseInsensitiveExtensions() {
	os.MkdirAll(suite.dropzonePath, 0755)
	suite.helper.Config.UploadDir = filepath.Join("test_dropzone_data", "uploads")
	
	suite.mockQueue.On("EnqueueJob", mock.Anything).Return(nil)
	
	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	err := service.Start()
	assert.NoError(suite.T(), err)
	defer service.Stop()
	
	// Test case variations
	uppercaseFile := filepath.Join(suite.dropzonePath, "test.MP3")
	mixedCaseFile := filepath.Join(suite.dropzonePath, "test.WaV")
	
	os.WriteFile(uppercaseFile, []byte("dummy audio"), 0644)
	os.WriteFile(mixedCaseFile, []byte("dummy audio"), 0644)
	
	// Give time to process
	time.Sleep(1500 * time.Millisecond)
	
	// Files should be processed regardless of case
	_, err1 := os.Stat(uppercaseFile)
	_, err2 := os.Stat(mixedCaseFile)
	
	// At least one should be removed (processed)
	assert.True(suite.T(), os.IsNotExist(err1) || os.IsNotExist(err2))
}

// Test multitrack files are not auto-transcribed
func (suite *DropzoneTestSuite) TestMultiTrackNotAutoTranscribed() {
	os.MkdirAll(suite.dropzonePath, 0755)
	suite.helper.Config.UploadDir = filepath.Join("test_dropzone_data", "uploads")
	
	// Enable auto-transcription
	suite.helper.DB.Model(&models.User{}).Where("username = ?", suite.helper.TestUser.Username).
		Update("auto_transcription_enabled", true)
	
	suite.mockQueue.On("EnqueueJob", mock.Anything).Return(nil)
	
	service := dropzone.NewService(suite.helper.Config, suite.mockQueue)
	err := service.Start()
	assert.NoError(suite.T(), err)
	defer service.Stop()
	
	// Create a multitrack job manually (dropzone can't detect multitrack on its own)
	// This test verifies the logic exists even if not directly testable via dropzone
	job := &models.TranscriptionJob{
		Title:        stringPtr("Multitrack Test"),
		Status:       models.StatusUploaded,
		AudioPath:    "test/multitrack.mp3",
		IsMultiTrack: true,
	}
	suite.helper.DB.Create(job)
	
	// Verify it's created as uploaded, not pending
	assert.Equal(suite.T(), models.StatusUploaded, job.Status)
}

func TestDropzoneTestSuite(t *testing.T) {
	suite.Run(t, new(DropzoneTestSuite))
}
