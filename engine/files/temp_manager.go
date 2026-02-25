package files

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TempFileInfo stores metadata about a temporary file
type TempFileInfo struct {
	Path      string
	ExpiresAt time.Time
	CompanyID string
}

// TempFileManager manages temporary files with automatic cleanup based on retention
type TempFileManager struct {
	baseDir string
	files   map[string]*TempFileInfo // path -> info
	mu      sync.RWMutex
	ticker  *time.Ticker
	done    chan struct{}
}

// NewTempFileManager creates a new TempFileManager
// dataDir is the base data directory (e.g., /data from CONNECTAI_DATA_DIR)
func NewTempFileManager(dataDir string) *TempFileManager {
	if dataDir == "" {
		dataDir = "/data"
	}
	baseDir := filepath.Join(dataDir, "temp")

	return &TempFileManager{
		baseDir: baseDir,
		files:   make(map[string]*TempFileInfo),
		done:    make(chan struct{}),
	}
}

// SaveTempFile saves bytes to a temp file with the specified retention period
// Returns the full path to the saved file
func (m *TempFileManager) SaveTempFile(companyID, filename string, data []byte, retentionSeconds int) (string, error) {
	// Create company-specific directory
	companyDir := filepath.Join(m.baseDir, companyID)
	if err := os.MkdirAll(companyDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Generate unique filename with timestamp to avoid collisions
	timestamp := time.Now().UnixNano()
	uniqueFilename := fmt.Sprintf("%d_%s", timestamp, filename)
	filePath := filepath.Join(companyDir, uniqueFilename)

	// Write file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	// Calculate expiration time
	expiresAt := time.Now().Add(time.Duration(retentionSeconds) * time.Second)

	// Store metadata
	m.mu.Lock()
	m.files[filePath] = &TempFileInfo{
		Path:      filePath,
		ExpiresAt: expiresAt,
		CompanyID: companyID,
	}
	m.mu.Unlock()

	return filePath, nil
}

// GetTempFile reads and returns the contents of a temp file
func (m *TempFileManager) GetTempFile(path string) ([]byte, error) {
	m.mu.RLock()
	info, exists := m.files[path]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("file not found or expired: %s", path)
	}

	// Check if file has expired
	if time.Now().After(info.ExpiresAt) {
		// Clean up expired file
		m.mu.Lock()
		delete(m.files, path)
		m.mu.Unlock()
		_ = os.Remove(path) // Best effort cleanup
		return nil, fmt.Errorf("file expired: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp file: %w", err)
	}

	return data, nil
}

// GetTempFileBase64 reads a temp file and returns it as base64-encoded string
func (m *TempFileManager) GetTempFileBase64(path string) (string, error) {
	data, err := m.GetTempFile(path)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// IsFileTracked checks if a file path is being tracked (and not expired)
func (m *TempFileManager) IsFileTracked(path string) bool {
	m.mu.RLock()
	info, exists := m.files[path]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	return time.Now().Before(info.ExpiresAt)
}

// StartCleanup starts the periodic cleanup goroutine
func (m *TempFileManager) StartCleanup(interval time.Duration) {
	m.ticker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-m.ticker.C:
				m.cleanup()
			case <-m.done:
				return
			}
		}
	}()
}

// Stop stops the cleanup goroutine
func (m *TempFileManager) Stop() {
	if m.ticker != nil {
		m.ticker.Stop()
	}
	close(m.done)
}

// cleanup removes expired files
func (m *TempFileManager) cleanup() {
	now := time.Now()
	var expiredPaths []string

	// Find expired files
	m.mu.RLock()
	for path, info := range m.files {
		if now.After(info.ExpiresAt) {
			expiredPaths = append(expiredPaths, path)
		}
	}
	m.mu.RUnlock()

	// Remove expired files
	if len(expiredPaths) > 0 {
		m.mu.Lock()
		for _, path := range expiredPaths {
			delete(m.files, path)
			_ = os.Remove(path) // Best effort removal
		}
		m.mu.Unlock()
	}

	// Also clean up any orphaned files on disk (files without metadata)
	m.cleanupOrphanedFiles()
}

// cleanupOrphanedFiles removes files from disk that are no longer tracked
// This handles cases where the application crashed before cleanup
func (m *TempFileManager) cleanupOrphanedFiles() {
	// Walk the temp directory
	err := filepath.Walk(m.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			return nil // Skip directories
		}

		// Check if file is tracked
		m.mu.RLock()
		_, tracked := m.files[path]
		m.mu.RUnlock()

		if !tracked {
			// File exists on disk but not tracked - check modification time
			// If file is older than default retention, remove it
			if time.Since(info.ModTime()) > 10*time.Minute {
				_ = os.Remove(path) // Best effort removal
			}
		}

		return nil
	})

	if err != nil {
		// Log error but don't fail
		return
	}
}

// GetFileCount returns the number of tracked files (for testing/monitoring)
func (m *TempFileManager) GetFileCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.files)
}
