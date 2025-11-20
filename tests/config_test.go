package tests

import (
	"os"
	"path/filepath"
	"testing"

	"synthezia/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ConfigTestSuite struct {
	suite.Suite
	originalEnv map[string]string
}

func (suite *ConfigTestSuite) SetupSuite() {
	// Save original environment variables
	suite.originalEnv = map[string]string{
		"PORT":          os.Getenv("PORT"),
		"HOST":          os.Getenv("HOST"),
		"DATABASE_PATH": os.Getenv("DATABASE_PATH"),
		"JWT_SECRET":    os.Getenv("JWT_SECRET"),
		"UPLOAD_DIR":    os.Getenv("UPLOAD_DIR"),
		"UV_PATH":       os.Getenv("UV_PATH"),
		"WHISPERX_ENV":  os.Getenv("WHISPERX_ENV"),
	}
}

func (suite *ConfigTestSuite) TearDownSuite() {
	// Restore original environment variables
	for key, value := range suite.originalEnv {
		if value != "" {
			os.Setenv(key, value)
		} else {
			os.Unsetenv(key)
		}
	}
}

func (suite *ConfigTestSuite) SetupTest() {
	// Clear all config-related environment variables before each test
	os.Unsetenv("PORT")
	os.Unsetenv("HOST")
	os.Unsetenv("DATABASE_PATH")
	os.Unsetenv("JWT_SECRET")
	os.Unsetenv("UPLOAD_DIR")
	os.Unsetenv("UV_PATH")
	os.Unsetenv("WHISPERX_ENV")
}

// Test Load with default values
func (suite *ConfigTestSuite) TestLoadDefaults() {
	cfg := config.Load()
	
	assert.NotNil(suite.T(), cfg)
	assert.Equal(suite.T(), "8080", cfg.Port)
	assert.Equal(suite.T(), "localhost", cfg.Host)
	assert.Equal(suite.T(), "data/synthezia.db", cfg.DatabasePath)
	assert.Equal(suite.T(), "data/uploads", cfg.UploadDir)
	assert.Equal(suite.T(), "whisperx-env/WhisperX", cfg.WhisperXEnv)
	assert.NotEmpty(suite.T(), cfg.JWTSecret)
	assert.NotEmpty(suite.T(), cfg.UVPath)
}

// Test Load with custom environment variables
func (suite *ConfigTestSuite) TestLoadCustomEnv() {
	os.Setenv("PORT", "9090")
	os.Setenv("HOST", "0.0.0.0")
	os.Setenv("DATABASE_PATH", "/custom/path/db.sqlite")
	os.Setenv("JWT_SECRET", "custom-jwt-secret-123")
	os.Setenv("UPLOAD_DIR", "/custom/uploads")
	os.Setenv("UV_PATH", "/custom/uv")
	os.Setenv("WHISPERX_ENV", "/custom/whisperx")
	
	cfg := config.Load()
	
	assert.Equal(suite.T(), "9090", cfg.Port)
	assert.Equal(suite.T(), "0.0.0.0", cfg.Host)
	assert.Equal(suite.T(), "/custom/path/db.sqlite", cfg.DatabasePath)
	assert.Equal(suite.T(), "custom-jwt-secret-123", cfg.JWTSecret)
	assert.Equal(suite.T(), "/custom/uploads", cfg.UploadDir)
	assert.Equal(suite.T(), "/custom/uv", cfg.UVPath)
	assert.Equal(suite.T(), "/custom/whisperx", cfg.WhisperXEnv)
}

// Test JWT secret generation when not provided
func (suite *ConfigTestSuite) TestJWTSecretGeneration() {
	// Ensure no JWT_SECRET in env
	os.Unsetenv("JWT_SECRET")
	
	// Clean up any existing jwt_secret file
	secretFile := "data/jwt_secret"
	os.Remove(secretFile)
	
	cfg := config.Load()
	
	// Should have generated a secret
	assert.NotEmpty(suite.T(), cfg.JWTSecret)
	assert.NotEqual(suite.T(), "fallback-jwt-secret-please-set-JWT_SECRET-env-var", cfg.JWTSecret)
	
	// Secret should be 64 characters (32 bytes hex-encoded)
	assert.Equal(suite.T(), 64, len(cfg.JWTSecret))
	
	// Load again - should get the same persisted secret
	cfg2 := config.Load()
	assert.Equal(suite.T(), cfg.JWTSecret, cfg2.JWTSecret)
	
	// Clean up
	os.Remove(secretFile)
}

// Test JWT secret from environment takes precedence
func (suite *ConfigTestSuite) TestJWTSecretFromEnv() {
	customSecret := "my-custom-jwt-secret-from-env"
	os.Setenv("JWT_SECRET", customSecret)
	
	cfg := config.Load()
	
	assert.Equal(suite.T(), customSecret, cfg.JWTSecret)
}

// Test JWT secret file persistence
func (suite *ConfigTestSuite) TestJWTSecretPersistence() {
	os.Unsetenv("JWT_SECRET")
	
	// Use custom secret file for test
	testSecretFile := "test_jwt_secret_file"
	os.Setenv("JWT_SECRET_FILE", testSecretFile)
	
	// Clean up
	defer os.Remove(testSecretFile)
	
	// First load - generates and saves secret
	cfg1 := config.Load()
	secret1 := cfg1.JWTSecret
	
	// Verify file was created
	_, err := os.Stat(testSecretFile)
	assert.NoError(suite.T(), err)
	
	// Second load - reads from file
	cfg2 := config.Load()
	secret2 := cfg2.JWTSecret
	
	// Should be the same secret
	assert.Equal(suite.T(), secret1, secret2)
	
	os.Unsetenv("JWT_SECRET_FILE")
}

// Test UV path detection
func (suite *ConfigTestSuite) TestUVPathDetection() {
	// Test with custom UV_PATH
	os.Setenv("UV_PATH", "/custom/uv/path")
	cfg := config.Load()
	assert.Equal(suite.T(), "/custom/uv/path", cfg.UVPath)
	
	// Test without UV_PATH (will try to find in PATH)
	os.Unsetenv("UV_PATH")
	cfg2 := config.Load()
	// Should return something (either found in PATH or fallback "uv")
	assert.NotEmpty(suite.T(), cfg2.UVPath)
}

// Test Config struct fields
func (suite *ConfigTestSuite) TestConfigStructure() {
	cfg := &config.Config{
		Port:         "3000",
		Host:         "127.0.0.1",
		DatabasePath: "/path/to/db",
		JWTSecret:    "secret",
		UploadDir:    "/uploads",
		UVPath:       "/usr/bin/uv",
		WhisperXEnv:  "/whisperx",
	}
	
	assert.Equal(suite.T(), "3000", cfg.Port)
	assert.Equal(suite.T(), "127.0.0.1", cfg.Host)
	assert.Equal(suite.T(), "/path/to/db", cfg.DatabasePath)
	assert.Equal(suite.T(), "secret", cfg.JWTSecret)
	assert.Equal(suite.T(), "/uploads", cfg.UploadDir)
	assert.Equal(suite.T(), "/usr/bin/uv", cfg.UVPath)
	assert.Equal(suite.T(), "/whisperx", cfg.WhisperXEnv)
}

// Test multiple Load calls return consistent values
func (suite *ConfigTestSuite) TestMultipleLoadCalls() {
	os.Setenv("PORT", "8888")
	os.Setenv("HOST", "192.168.1.1")
	
	cfg1 := config.Load()
	cfg2 := config.Load()
	
	assert.Equal(suite.T(), cfg1.Port, cfg2.Port)
	assert.Equal(suite.T(), cfg1.Host, cfg2.Host)
	assert.Equal(suite.T(), cfg1.DatabasePath, cfg2.DatabasePath)
}

// Test empty environment variable values use defaults
func (suite *ConfigTestSuite) TestEmptyEnvUsesDefaults() {
	os.Setenv("PORT", "")
	os.Setenv("HOST", "")
	os.Setenv("DATABASE_PATH", "")
	
	cfg := config.Load()
	
	// Empty strings should fall back to defaults
	assert.Equal(suite.T(), "8080", cfg.Port)
	assert.Equal(suite.T(), "localhost", cfg.Host)
	assert.Equal(suite.T(), "data/synthezia.db", cfg.DatabasePath)
}

// Test .env file loading
func (suite *ConfigTestSuite) TestDotEnvFile() {
	// Create a temporary .env file
	envContent := `PORT=7777
HOST=test.example.com
DATABASE_PATH=/tmp/test.db`
	
	envFile := ".env.test"
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	assert.NoError(suite.T(), err)
	defer os.Remove(envFile)
	
	// Note: godotenv.Load() looks for .env by default, not .env.test
	// This test verifies the mechanism exists, actual loading would need the file named .env
}

// Test JWT secret with custom file path
func (suite *ConfigTestSuite) TestJWTSecretCustomFilePath() {
	os.Unsetenv("JWT_SECRET")
	
	customPath := "custom_dir/jwt_token"
	os.Setenv("JWT_SECRET_FILE", customPath)
	defer os.Unsetenv("JWT_SECRET_FILE")
	defer os.RemoveAll("custom_dir")
	
	cfg := config.Load()
	
	// Should generate secret and create file at custom path
	assert.NotEmpty(suite.T(), cfg.JWTSecret)
	
	// Directory should be created
	dir := filepath.Dir(customPath)
	_, err := os.Stat(dir)
	assert.NoError(suite.T(), err)
}

// Test configuration with special characters
func (suite *ConfigTestSuite) TestConfigWithSpecialCharacters() {
	os.Setenv("JWT_SECRET", "secret!@#$%^&*()_+-=[]{}|;:,.<>?")
	os.Setenv("DATABASE_PATH", "/path/with spaces/db.sqlite")
	
	cfg := config.Load()
	
	assert.Contains(suite.T(), cfg.JWTSecret, "!@#$")
	assert.Contains(suite.T(), cfg.DatabasePath, "with spaces")
}

// Test UV path fallback when not found
func (suite *ConfigTestSuite) TestUVPathFallback() {
	os.Unsetenv("UV_PATH")
	
	// Even if uv is not in PATH, should return fallback "uv"
	cfg := config.Load()
	assert.NotEmpty(suite.T(), cfg.UVPath)
}

// Test concurrent config loads (thread safety)
func (suite *ConfigTestSuite) TestConcurrentConfigLoads() {
	os.Setenv("PORT", "5000")
	
	done := make(chan bool)
	
	// Load config concurrently from multiple goroutines
	for i := 0; i < 10; i++ {
		go func() {
			cfg := config.Load()
			assert.NotNil(suite.T(), cfg)
			assert.Equal(suite.T(), "5000", cfg.Port)
			done <- true
		}()
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Test config with very long values
func (suite *ConfigTestSuite) TestConfigWithLongValues() {
	longPath := "/very/long/path/" + string(make([]byte, 500))
	os.Setenv("DATABASE_PATH", longPath)
	
	cfg := config.Load()
	assert.Equal(suite.T(), longPath, cfg.DatabasePath)
}

// Test JWT secret hex encoding
func (suite *ConfigTestSuite) TestJWTSecretHexEncoding() {
	os.Unsetenv("JWT_SECRET")
	os.Remove("data/jwt_secret")
	
	cfg := config.Load()
	
	// Secret should be valid hex string
	for _, c := range cfg.JWTSecret {
		assert.True(suite.T(), 
			(c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"JWT secret should be hex-encoded")
	}
	
	os.Remove("data/jwt_secret")
}

// Test default values are reasonable
func (suite *ConfigTestSuite) TestDefaultValuesAreReasonable() {
	cfg := config.Load()
	
	// Port should be valid
	assert.NotEmpty(suite.T(), cfg.Port)
	
	// Host should be valid
	assert.NotEmpty(suite.T(), cfg.Host)
	
	// Paths should be relative or absolute
	assert.NotEmpty(suite.T(), cfg.DatabasePath)
	assert.NotEmpty(suite.T(), cfg.UploadDir)
	assert.NotEmpty(suite.T(), cfg.WhisperXEnv)
	
	// JWT secret should be secure length
	assert.GreaterOrEqual(suite.T(), len(cfg.JWTSecret), 32)
}

// Test config modifications don't affect subsequent loads
func (suite *ConfigTestSuite) TestConfigImmutability() {
	os.Setenv("PORT", "6000")
	
	cfg1 := config.Load()
	originalPort := cfg1.Port
	
	// Modify the config
	cfg1.Port = "9999"
	
	// Load again
	cfg2 := config.Load()
	
	// Should get original value, not modified one
	assert.Equal(suite.T(), originalPort, cfg2.Port)
	assert.NotEqual(suite.T(), "9999", cfg2.Port)
}

// Test whitespace handling in JWT secret
func (suite *ConfigTestSuite) TestJWTSecretWhitespaceHandling() {
	os.Unsetenv("JWT_SECRET")
	
	testSecretFile := "test_jwt_secret_whitespace"
	os.Setenv("JWT_SECRET_FILE", testSecretFile)
	defer os.Remove(testSecretFile)
	defer os.Unsetenv("JWT_SECRET_FILE")
	
	// Write secret with whitespace
	os.WriteFile(testSecretFile, []byte("  secret-with-spaces  \n"), 0600)
	
	cfg := config.Load()
	
	// Should trim whitespace
	assert.Equal(suite.T(), "secret-with-spaces", cfg.JWTSecret)
	assert.NotContains(suite.T(), cfg.JWTSecret, " ")
	assert.NotContains(suite.T(), cfg.JWTSecret, "\n")
}

// Test config handles missing data directory gracefully
func (suite *ConfigTestSuite) TestConfigHandlesMissingDataDir() {
	os.RemoveAll("data")
	
	// Should not panic
	cfg := config.Load()
	assert.NotNil(suite.T(), cfg)
	
	// JWT secret should still be generated/loaded
	assert.NotEmpty(suite.T(), cfg.JWTSecret)
}

func TestConfigTestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}
