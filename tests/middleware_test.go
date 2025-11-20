package tests

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"synthezia/internal/auth"
	"synthezia/pkg/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type MiddlewareTestSuite struct {
	suite.Suite
	helper      *TestHelper
	authService *auth.AuthService
}

func (suite *MiddlewareTestSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)
	suite.helper = NewTestHelper(suite.T(), "middleware_test.db")
	suite.authService = suite.helper.AuthService
}

func (suite *MiddlewareTestSuite) TearDownSuite() {
	suite.helper.Cleanup()
}

// Test AuthMiddleware with valid JWT
func (suite *MiddlewareTestSuite) TestAuthMiddlewareValidJWT() {
	router := gin.New()
	router.Use(middleware.AuthMiddleware(suite.authService))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+suite.helper.TestToken)
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
}

// Test AuthMiddleware with invalid JWT
func (suite *MiddlewareTestSuite) TestAuthMiddlewareInvalidJWT() {
	router := gin.New()
	router.Use(middleware.AuthMiddleware(suite.authService))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusUnauthorized, w.Code)
}

// Test AuthMiddleware with valid API key
func (suite *MiddlewareTestSuite) TestAuthMiddlewareValidAPIKey() {
	router := gin.New()
	router.Use(middleware.AuthMiddleware(suite.authService))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("X-API-Key", suite.helper.TestAPIKey)
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
}

// Test AuthMiddleware with invalid API key
func (suite *MiddlewareTestSuite) TestAuthMiddlewareInvalidAPIKey() {
	router := gin.New()
	router.Use(middleware.AuthMiddleware(suite.authService))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("X-API-Key", "invalid-api-key")
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusUnauthorized, w.Code)
}

// Test AuthMiddleware with no authentication
func (suite *MiddlewareTestSuite) TestAuthMiddlewareNoAuth() {
	router := gin.New()
	router.Use(middleware.AuthMiddleware(suite.authService))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/protected", nil)
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusUnauthorized, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "Missing authentication")
}

// Test AuthMiddleware with malformed authorization header
func (suite *MiddlewareTestSuite) TestAuthMiddlewareMalformedHeader() {
	router := gin.New()
	router.Use(middleware.AuthMiddleware(suite.authService))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Missing "Bearer" prefix
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "token-without-bearer")
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusUnauthorized, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "Invalid authorization header format")
}

// Test AuthMiddleware prefers API key over JWT
func (suite *MiddlewareTestSuite) TestAuthMiddlewareAPIKeyPrecedence() {
	router := gin.New()
	router.Use(middleware.AuthMiddleware(suite.authService))
	router.GET("/protected", func(c *gin.Context) {
		authType, _ := c.Get("auth_type")
		c.JSON(http.StatusOK, gin.H{"auth_type": authType})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("X-API-Key", suite.helper.TestAPIKey)
	req.Header.Set("Authorization", "Bearer "+suite.helper.TestToken)
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "api_key")
}

// Test JWTOnlyMiddleware with valid JWT
func (suite *MiddlewareTestSuite) TestJWTOnlyMiddlewareValid() {
	router := gin.New()
	router.Use(middleware.JWTOnlyMiddleware(suite.authService))
	router.GET("/jwt-only", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/jwt-only", nil)
	req.Header.Set("Authorization", "Bearer "+suite.helper.TestToken)
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
}

// Test JWTOnlyMiddleware rejects API key
func (suite *MiddlewareTestSuite) TestJWTOnlyMiddlewareRejectsAPIKey() {
	router := gin.New()
	router.Use(middleware.JWTOnlyMiddleware(suite.authService))
	router.GET("/jwt-only", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/jwt-only", nil)
	req.Header.Set("X-API-Key", suite.helper.TestAPIKey)
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusUnauthorized, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "Authorization header required")
}

// Test JWTOnlyMiddleware with no auth
func (suite *MiddlewareTestSuite) TestJWTOnlyMiddlewareNoAuth() {
	router := gin.New()
	router.Use(middleware.JWTOnlyMiddleware(suite.authService))
	router.GET("/jwt-only", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/jwt-only", nil)
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusUnauthorized, w.Code)
}

// Test APIKeyOnlyMiddleware with valid key
func (suite *MiddlewareTestSuite) TestAPIKeyOnlyMiddlewareValid() {
	router := gin.New()
	router.Use(middleware.APIKeyOnlyMiddleware())
	router.GET("/api-key-only", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api-key-only", nil)
	req.Header.Set("X-API-Key", suite.helper.TestAPIKey)
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
}

// Test APIKeyOnlyMiddleware rejects JWT
func (suite *MiddlewareTestSuite) TestAPIKeyOnlyMiddlewareRejectsJWT() {
	router := gin.New()
	router.Use(middleware.APIKeyOnlyMiddleware())
	router.GET("/api-key-only", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api-key-only", nil)
	req.Header.Set("Authorization", "Bearer "+suite.helper.TestToken)
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusUnauthorized, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "API key required")
}

// Test CompressionMiddleware compresses JSON responses
func (suite *MiddlewareTestSuite) TestCompressionMiddlewareJSON() {
	router := gin.New()
	router.Use(middleware.CompressionMiddleware())
	router.GET("/json", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "test response with compression"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/json", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Equal(suite.T(), "gzip", w.Header().Get("Content-Encoding"))
	assert.Equal(suite.T(), "Accept-Encoding", w.Header().Get("Vary"))

	// Verify content is actually gzipped
	reader, err := gzip.NewReader(w.Body)
	assert.NoError(suite.T(), err)
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	assert.NoError(suite.T(), err)
	assert.Contains(suite.T(), string(decompressed), "test response")
}

// Test CompressionMiddleware skips when client doesn't accept gzip
func (suite *MiddlewareTestSuite) TestCompressionMiddlewareNoAcceptEncoding() {
	router := gin.New()
	router.Use(middleware.CompressionMiddleware())
	router.GET("/json", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "test"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/json", nil)
	// No Accept-Encoding header
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Empty(suite.T(), w.Header().Get("Content-Encoding"))
}

// Test CompressionMiddleware with different compression levels
func (suite *MiddlewareTestSuite) TestCompressionMiddlewareWithLevel() {
	router := gin.New()
	router.Use(middleware.CompressionMiddlewareWithLevel(gzip.BestSpeed))
	router.GET("/json", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": strings.Repeat("test", 100)})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/json", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Equal(suite.T(), "gzip", w.Header().Get("Content-Encoding"))
}

// Test CompressionMiddleware skips HEAD requests
func (suite *MiddlewareTestSuite) TestCompressionMiddlewareSkipsHEAD() {
	router := gin.New()
	router.Use(middleware.CompressionMiddleware())
	router.HEAD("/json", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("HEAD", "/json", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Empty(suite.T(), w.Header().Get("Content-Encoding"))
}

// Test CompressionMiddleware skips streaming responses
func (suite *MiddlewareTestSuite) TestCompressionMiddlewareSkipsStreaming() {
	router := gin.New()
	router.Use(middleware.CompressionMiddleware())
	router.GET("/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK, "data: test\n\n")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/stream", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Empty(suite.T(), w.Header().Get("Content-Encoding"))
}

// Test NoCompressionMiddleware sets header
func (suite *MiddlewareTestSuite) TestNoCompressionMiddleware() {
	router := gin.New()
	router.Use(middleware.NoCompressionMiddleware())
	router.GET("/no-compression", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "test"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/no-compression", nil)
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Equal(suite.T(), "1", w.Header().Get("X-No-Compression"))
}

// Test CompressionMiddleware with HTML content
func (suite *MiddlewareTestSuite) TestCompressionMiddlewareHTML() {
	router := gin.New()
	router.Use(middleware.CompressionMiddleware())
	router.GET("/html", func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.String(http.StatusOK, "<html><body>Test</body></html>")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/html", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Equal(suite.T(), "gzip", w.Header().Get("Content-Encoding"))
}

// Test CompressionMiddleware skips binary content
func (suite *MiddlewareTestSuite) TestCompressionMiddlewareSkipsBinary() {
	router := gin.New()
	router.Use(middleware.CompressionMiddleware())
	router.GET("/binary", func(c *gin.Context) {
		c.Header("Content-Type", "application/octet-stream")
		c.String(http.StatusOK, "binary data")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/binary", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Empty(suite.T(), w.Header().Get("Content-Encoding"))
}

// Test middleware chain with multiple middlewares
func (suite *MiddlewareTestSuite) TestMiddlewareChain() {
	router := gin.New()
	router.Use(middleware.CompressionMiddleware())
	router.Use(middleware.AuthMiddleware(suite.authService))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+suite.helper.TestToken)
	req.Header.Set("Accept-Encoding", "gzip")
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Equal(suite.T(), "gzip", w.Header().Get("Content-Encoding"))
}

// Test AuthMiddleware sets context values
func (suite *MiddlewareTestSuite) TestAuthMiddlewareSetsContext() {
	router := gin.New()
	router.Use(middleware.AuthMiddleware(suite.authService))
	router.GET("/check-context", func(c *gin.Context) {
		authType, _ := c.Get("auth_type")
		userID, userIDExists := c.Get("user_id")
		username, usernameExists := c.Get("username")
		c.JSON(http.StatusOK, gin.H{
			"auth_type":       authType,
			"user_id_exists":  userIDExists,
			"username_exists": usernameExists,
			"user_id":         userID,
			"username":        username,
		})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/check-context", nil)
	req.Header.Set("Authorization", "Bearer "+suite.helper.TestToken)
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "jwt")
	assert.Contains(suite.T(), w.Body.String(), suite.helper.TestUser.Username)
}

// Test CompressionMiddleware with WebSocket upgrade
func (suite *MiddlewareTestSuite) TestCompressionMiddlewareSkipsWebSocket() {
	router := gin.New()
	router.Use(middleware.CompressionMiddleware())
	router.GET("/ws", func(c *gin.Context) {
		c.String(http.StatusOK, "websocket")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ws", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)
	assert.Empty(suite.T(), w.Header().Get("Content-Encoding"))
}

// Test AuthMiddleware handles empty Bearer token
func (suite *MiddlewareTestSuite) TestAuthMiddlewareEmptyBearerToken() {
	router := gin.New()
	router.Use(middleware.AuthMiddleware(suite.authService))
	router.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer ")
	router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusUnauthorized, w.Code)
}

func TestMiddlewareTestSuite(t *testing.T) {
	suite.Run(t, new(MiddlewareTestSuite))
}
