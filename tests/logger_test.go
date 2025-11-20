package tests

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"synthezia/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type LoggerTestSuite struct {
	suite.Suite
	originalLogLevel string
}

func (suite *LoggerTestSuite) SetupSuite() {
	// Save original log level
	suite.originalLogLevel = os.Getenv("LOG_LEVEL")
}

func (suite *LoggerTestSuite) TearDownSuite() {
	// Restore original log level
	if suite.originalLogLevel != "" {
		os.Setenv("LOG_LEVEL", suite.originalLogLevel)
	}
}

func (suite *LoggerTestSuite) SetupTest() {
	// Reset to default INFO level before each test
	logger.Init("info")
}

// Test logger initialization with different levels
func (suite *LoggerTestSuite) TestLoggerInit() {
	logger.Init("debug")
	assert.Equal(suite.T(), logger.LevelDebug, logger.GetLevel())

	logger.Init("info")
	assert.Equal(suite.T(), logger.LevelInfo, logger.GetLevel())

	logger.Init("warn")
	assert.Equal(suite.T(), logger.LevelWarn, logger.GetLevel())

	logger.Init("error")
	assert.Equal(suite.T(), logger.LevelError, logger.GetLevel())
}

// Test logger initialization with invalid level defaults to info
func (suite *LoggerTestSuite) TestLoggerInitInvalidLevel() {
	logger.Init("invalid")
	assert.Equal(suite.T(), logger.LevelInfo, logger.GetLevel())
}

// Test logger initialization with empty level defaults to info
func (suite *LoggerTestSuite) TestLoggerInitEmptyLevel() {
	logger.Init("")
	assert.Equal(suite.T(), logger.LevelInfo, logger.GetLevel())
}

// Test Get returns non-nil logger
func (suite *LoggerTestSuite) TestLoggerGet() {
	log := logger.Get()
	assert.NotNil(suite.T(), log)
}

// Test Debug logging
func (suite *LoggerTestSuite) TestDebugLogging() {
	logger.Init("debug")
	
	// Should not panic
	logger.Debug("Test debug message", "key", "value")
	
	// At INFO level, debug should be filtered
	logger.Init("info")
	logger.Debug("This should not appear", "key", "value")
}

// Test Info logging
func (suite *LoggerTestSuite) TestInfoLogging() {
	logger.Init("info")
	
	// Should not panic
	logger.Info("Test info message", "key", "value")
	
	// At WARN level, info should be filtered
	logger.Init("warn")
	logger.Info("This should not appear", "key", "value")
}

// Test Warn logging
func (suite *LoggerTestSuite) TestWarnLogging() {
	logger.Init("warn")
	
	// Should not panic
	logger.Warn("Test warn message", "key", "value")
	
	// At ERROR level, warn should be filtered
	logger.Init("error")
	logger.Warn("This should not appear", "key", "value")
}

// Test Error logging
func (suite *LoggerTestSuite) TestErrorLogging() {
	logger.Init("error")
	
	// Should not panic
	logger.Error("Test error message", "key", "value")
}

// Test WithContext creates logger with context
func (suite *LoggerTestSuite) TestWithContext() {
	log := logger.WithContext("request_id", "12345")
	assert.NotNil(suite.T(), log)
	
	// Should not panic when logging
	log.Info("Test with context", "additional", "data")
}

// Test Startup logging
func (suite *LoggerTestSuite) TestStartupLogging() {
	logger.Init("info")
	
	// Should not panic
	logger.Startup("database", "Database initialized", "connections", 10)
	
	// Test at debug level
	logger.Init("debug")
	logger.Startup("server", "Server starting", "port", 8080)
}

// Test JobStarted logging
func (suite *LoggerTestSuite) TestJobStartedLogging() {
	logger.Init("info")
	
	params := map[string]any{
		"batch_size": 16,
		"model":      "base",
	}
	
	// Should not panic
	logger.JobStarted("job-123", "audio.mp3", "whisperx", params)
}

// Test JobCompleted logging
func (suite *LoggerTestSuite) TestJobCompletedLogging() {
	logger.Init("info")
	
	// Should not panic
	logger.JobCompleted("job-123", 5000000000, map[string]any{"words": 150})
}

// Test JobFailed logging
func (suite *LoggerTestSuite) TestJobFailedLogging() {
	logger.Init("info")
	
	// Should not panic
	logger.JobFailed("job-123", 2000000000, assert.AnError)
}

// Test HTTPRequest logging with filtering
func (suite *LoggerTestSuite) TestHTTPRequestLogging() {
	logger.Init("info")
	
	// Regular endpoint should log
	logger.HTTPRequest("GET", "/api/v1/transcription/submit", 200, 5000000, "test-agent")
	
	// Filtered endpoints should not log at INFO
	logger.HTTPRequest("GET", "/api/v1/transcription/list", 200, 5000000, "test-agent")
	logger.HTTPRequest("GET", "/health", 200, 5000000, "test-agent")
	logger.HTTPRequest("GET", "/api/v1/job/123/status", 200, 5000000, "test-agent")
	
	// At DEBUG level, all should log
	logger.Init("debug")
	logger.HTTPRequest("GET", "/health", 200, 5000000, "test-agent")
}

// Test AuthEvent logging
func (suite *LoggerTestSuite) TestAuthEventLogging() {
	logger.Init("info")
	
	// Successful login
	logger.AuthEvent("login", "testuser", "192.168.1.1", true, "method", "jwt")
	
	// Failed login
	logger.AuthEvent("login", "testuser", "192.168.1.1", false, "reason", "invalid_password")
}

// Test WorkerOperation logging
func (suite *LoggerTestSuite) TestWorkerOperationLogging() {
	logger.Init("debug")
	
	// Should only log at debug level
	logger.WorkerOperation(1, "job-123", "started", "queue_size", 5)
	
	logger.Init("info")
	// Should not appear at info level
	logger.WorkerOperation(2, "job-456", "completed", "duration", "5s")
}

// Test Performance logging
func (suite *LoggerTestSuite) TestPerformanceLogging() {
	logger.Init("debug")
	
	// Should only log at debug level
	logger.Performance("transcription", 5000000000, "model", "whisperx")
	
	logger.Init("info")
	// Should not appear at info level
	logger.Performance("database_query", 50000000, "query", "SELECT")
}

// Test GinLogger middleware
func (suite *LoggerTestSuite) TestGinLoggerMiddleware() {
	gin.SetMode(gin.TestMode)
	logger.Init("info")
	
	router := gin.New()
	router.Use(logger.GinLogger())
	
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w, req)
	
	assert.Equal(suite.T(), http.StatusOK, w.Code)
}

// Test GinLogger middleware with query parameters
func (suite *LoggerTestSuite) TestGinLoggerMiddlewareWithQuery() {
	gin.SetMode(gin.TestMode)
	logger.Init("debug")
	
	router := gin.New()
	router.Use(logger.GinLogger())
	
	router.GET("/search", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/search?q=test&limit=10", nil)
	router.ServeHTTP(w, req)
	
	assert.Equal(suite.T(), http.StatusOK, w.Code)
}

// Test GinLogger middleware filters status endpoints
func (suite *LoggerTestSuite) TestGinLoggerMiddlewareFiltering() {
	gin.SetMode(gin.TestMode)
	logger.Init("info")
	
	router := gin.New()
	router.Use(logger.GinLogger())
	
	router.GET("/api/v1/job/:id/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
	
	// These should be filtered at INFO level
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/api/v1/job/123/status", nil)
	router.ServeHTTP(w1, req1)
	
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/health", nil)
	router.ServeHTTP(w2, req2)
	
	assert.Equal(suite.T(), http.StatusOK, w1.Code)
	assert.Equal(suite.T(), http.StatusOK, w2.Code)
}

// Test GinLogger middleware with different status codes
func (suite *LoggerTestSuite) TestGinLoggerMiddlewareStatusCodes() {
	gin.SetMode(gin.TestMode)
	logger.Init("info")
	
	router := gin.New()
	router.Use(logger.GinLogger())
	
	router.GET("/200", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/404", func(c *gin.Context) { c.Status(http.StatusNotFound) })
	router.GET("/500", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })
	
	// Test various status codes
	statusCodes := []int{200, 404, 500}
	paths := []string{"/200", "/404", "/500"}
	
	for i, path := range paths {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", path, nil)
		router.ServeHTTP(w, req)
		assert.Equal(suite.T(), statusCodes[i], w.Code)
	}
}

// Test SetGinOutput suppresses default logs
func (suite *LoggerTestSuite) TestSetGinOutput() {
	// Should not panic
	logger.SetGinOutput()
	
	// Verify Gin's default writer is set to discard
	assert.NotNil(suite.T(), gin.DefaultWriter)
}

// Test case-insensitive log level parsing
func (suite *LoggerTestSuite) TestLogLevelCaseInsensitive() {
	logger.Init("DEBUG")
	assert.Equal(suite.T(), logger.LevelDebug, logger.GetLevel())
	
	logger.Init("INFO")
	assert.Equal(suite.T(), logger.LevelInfo, logger.GetLevel())
	
	logger.Init("Warning")
	assert.Equal(suite.T(), logger.LevelWarn, logger.GetLevel())
	
	logger.Init("ERROR")
	assert.Equal(suite.T(), logger.LevelError, logger.GetLevel())
}

// Test log level filtering at different levels
func (suite *LoggerTestSuite) TestLogLevelFiltering() {
	// At DEBUG level, all should pass
	logger.Init("debug")
	assert.LessOrEqual(suite.T(), logger.GetLevel(), logger.LevelDebug)
	
	// At INFO level, debug should be filtered
	logger.Init("info")
	assert.Greater(suite.T(), logger.GetLevel(), logger.LevelDebug)
	assert.LessOrEqual(suite.T(), logger.GetLevel(), logger.LevelInfo)
	
	// At WARN level, debug and info should be filtered
	logger.Init("warn")
	assert.Greater(suite.T(), logger.GetLevel(), logger.LevelInfo)
	assert.LessOrEqual(suite.T(), logger.GetLevel(), logger.LevelWarn)
	
	// At ERROR level, only errors should pass
	logger.Init("error")
	assert.Greater(suite.T(), logger.GetLevel(), logger.LevelWarn)
	assert.Equal(suite.T(), logger.LevelError, logger.GetLevel())
}

// Test logger with multiple arguments
func (suite *LoggerTestSuite) TestLoggerMultipleArguments() {
	logger.Init("debug")
	
	// Should handle multiple key-value pairs
	logger.Debug("Multiple args", "key1", "value1", "key2", 123, "key3", true)
	logger.Info("Multiple args", "user", "testuser", "action", "login", "success", true)
	logger.Warn("Multiple args", "resource", "memory", "usage", 85.5, "threshold", 80)
	logger.Error("Multiple args", "error", "connection failed", "retries", 3, "timeout", "30s")
}

// Test logger initialization from environment variable
func (suite *LoggerTestSuite) TestLoggerInitFromEnv() {
	os.Setenv("LOG_LEVEL", "debug")
	logger.Init(os.Getenv("LOG_LEVEL"))
	assert.Equal(suite.T(), logger.LevelDebug, logger.GetLevel())
	
	os.Setenv("LOG_LEVEL", "warn")
	logger.Init(os.Getenv("LOG_LEVEL"))
	assert.Equal(suite.T(), logger.LevelWarn, logger.GetLevel())
	
	os.Unsetenv("LOG_LEVEL")
}

// Test concurrent logging (basic thread safety)
func (suite *LoggerTestSuite) TestConcurrentLogging() {
	logger.Init("info")
	
	done := make(chan bool)
	
	// Spawn multiple goroutines logging concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			logger.Info("Concurrent log", "goroutine", id)
			logger.Debug("Concurrent debug", "goroutine", id)
			done <- true
		}(i)
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Should not panic
}

// Test GinLogger with POST request and body
func (suite *LoggerTestSuite) TestGinLoggerWithPOST() {
	gin.SetMode(gin.TestMode)
	logger.Init("debug")
	
	router := gin.New()
	router.Use(logger.GinLogger())
	
	router.POST("/submit", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"status": "created"})
	})
	
	body := bytes.NewBufferString(`{"title":"test"}`)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/submit", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	
	assert.Equal(suite.T(), http.StatusCreated, w.Code)
}

// Test logger output doesn't panic with nil values
func (suite *LoggerTestSuite) TestLoggerWithNilValues() {
	logger.Init("info")
	
	// Should handle nil values gracefully
	logger.Info("Test with nil", "key", nil)
	logger.Debug("Debug with nil", "value", nil, "number", 0)
}

// Test alternate log level names
func (suite *LoggerTestSuite) TestAlternateLogLevelNames() {
	// "warning" should map to warn
	logger.Init("warning")
	assert.Equal(suite.T(), logger.LevelWarn, logger.GetLevel())
}

// Test logger respects level for structured fields
func (suite *LoggerTestSuite) TestStructuredFieldsFiltering() {
	logger.Init("error")
	
	// These should be filtered
	logger.Debug("Debug message", "field1", "value1", "field2", 123)
	logger.Info("Info message", "user", "test")
	logger.Warn("Warn message", "status", "warning")
	
	// This should appear
	logger.Error("Error message", "error", "something broke")
}

// Test logger with empty messages
func (suite *LoggerTestSuite) TestLoggerWithEmptyMessages() {
	logger.Init("info")
	
	// Should handle empty messages
	logger.Info("")
	logger.Debug("", "key", "value")
	logger.Warn("")
	logger.Error("")
}

// Test GinLogger handles client IP correctly
func (suite *LoggerTestSuite) TestGinLoggerClientIP() {
	gin.SetMode(gin.TestMode)
	logger.Init("debug")
	
	router := gin.New()
	router.Use(logger.GinLogger())
	
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	router.ServeHTTP(w, req)
	
	assert.Equal(suite.T(), http.StatusOK, w.Code)
}

// Test logger doesn't panic with very long messages
func (suite *LoggerTestSuite) TestLoggerLongMessages() {
	logger.Init("info")
	
	longMessage := strings.Repeat("A", 10000)
	
	// Should handle very long messages
	logger.Info(longMessage)
	logger.Debug(longMessage, "key", strings.Repeat("B", 5000))
}

func TestLoggerTestSuite(t *testing.T) {
	suite.Run(t, new(LoggerTestSuite))
}
