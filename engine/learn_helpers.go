package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/henomis/lingoose/thread"
)

// MaxURLsPerLearnCall is the maximum number of URLs to process in a single Learn call.
const MaxURLsPerLearnCall = 3

// MaxFileSizeBytes is the maximum file size allowed for learning (50MB).
const MaxFileSizeBytes = 50 * 1024 * 1024

// ---------------------------------------------------------------------------
// Provider interfaces — injected by the host application.
// ---------------------------------------------------------------------------

// ContentDownloader downloads raw bytes from a URL and returns the MIME type.
type ContentDownloader interface {
	Download(ctx context.Context, url string) (mimeType string, data []byte, err error)
}

// PDFProcessor extracts text from PDF data.
type PDFProcessor interface {
	ExtractText(ctx context.Context, url string, data []byte) (string, error)
}

// ImageProcessor describes an image from raw bytes.
type ImageProcessor interface {
	Describe(ctx context.Context, url string, data []byte, mimeType string) (string, error)
}

// AudioTranscriber transcribes audio from raw bytes.
type AudioTranscriber interface {
	Transcribe(ctx context.Context, data []byte, mimeType string) (string, error)
}

// LearnProviders bundles all optional providers used by learn helpers.
type LearnProviders struct {
	Downloader ContentDownloader
	PDF        PDFProcessor
	Image      ImageProcessor
	Audio      AudioTranscriber
}

// package-level learn providers
var learnProviders LearnProviders

// SetLearnProviders configures the providers used by learn helpers.
func SetLearnProviders(p LearnProviders) {
	learnProviders = p
}

// ---------------------------------------------------------------------------
// LearnContentResult
// ---------------------------------------------------------------------------

// LearnContentResult holds the extracted content from a single URL.
type LearnContentResult struct {
	URL         string
	ContentType string // "pdf", "image", "audio", "text", "error"
	Content     string
	Error       error
}

// ---------------------------------------------------------------------------
// URL extraction (inline — replaces web.ExtractLinks)
// ---------------------------------------------------------------------------

var urlRegex = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `\]\)]+`)

// ExtractAndCategorizeURLs extracts URLs from text, removes them, and returns
// both the cleaned text and deduplicated URLs (limited to MaxURLsPerLearnCall).
func ExtractAndCategorizeURLs(text string) (cleanedText string, urls []string) {
	allURLs := urlRegex.FindAllString(text, -1)

	seen := make(map[string]bool)
	var uniqueURLs []string
	for _, u := range allURLs {
		if !seen[u] {
			seen[u] = true
			uniqueURLs = append(uniqueURLs, u)
		}
	}

	if len(uniqueURLs) > MaxURLsPerLearnCall {
		urls = uniqueURLs[:MaxURLsPerLearnCall]
	} else {
		urls = uniqueURLs
	}

	cleanedText = text
	for _, u := range uniqueURLs {
		cleanedText = strings.ReplaceAll(cleanedText, u, "")
	}

	cleanedText = strings.Join(strings.Fields(cleanedText), " ")
	cleanedText = strings.TrimSpace(cleanedText)

	return cleanedText, urls
}

// ---------------------------------------------------------------------------
// Content type detection
// ---------------------------------------------------------------------------

// ContentTypeInfo holds information detected from URL before downloading.
type ContentTypeInfo struct {
	Type          string // "pdf", "image", "audio", "video", "unknown"
	ContentLength int64  // -1 if unknown
}

// DetectContentTypeFromURL performs a HEAD request or uses URL extension
// to determine the content type BEFORE downloading.
func DetectContentTypeFromURL(ctx context.Context, urlStr string) (ContentTypeInfo, error) {
	info := ContentTypeInfo{Type: "unknown", ContentLength: -1}

	parsedURL, err := url.Parse(urlStr)
	if err == nil {
		ext := strings.ToLower(path.Ext(parsedURL.Path))
		switch ext {
		case ".pdf":
			info.Type = "pdf"
		case ".jpg", ".jpeg", ".png", ".gif", ".webp":
			info.Type = "image"
		case ".mp3", ".wav", ".ogg", ".m4a", ".aac", ".flac", ".opus", ".oga":
			info.Type = "audio"
		case ".mp4", ".avi", ".mov", ".mkv", ".webm", ".wmv", ".flv":
			info.Type = "video"
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "HEAD", urlStr, nil)
	if err != nil {
		return info, fmt.Errorf("failed to create HEAD request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return info, nil
	}
	defer resp.Body.Close()

	if resp.ContentLength > 0 {
		info.ContentLength = resp.ContentLength
	}

	if info.Type == "unknown" {
		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "pdf") {
			info.Type = "pdf"
		} else if strings.HasPrefix(contentType, "image/") {
			info.Type = "image"
		} else if strings.HasPrefix(contentType, "audio/") {
			info.Type = "audio"
		} else if strings.HasPrefix(contentType, "video/") {
			info.Type = "video"
		}
	}

	return info, nil
}

// ---------------------------------------------------------------------------
// Content processing
// ---------------------------------------------------------------------------

// ProcessPDFForLearning extracts and summarizes PDF content.
func ProcessPDFForLearning(ctx context.Context, urlStr string, data []byte) (string, error) {
	if learnProviders.PDF == nil {
		return "", fmt.Errorf("PDF processor not configured")
	}
	return learnProviders.PDF.ExtractText(ctx, urlStr, data)
}

// ProcessImageForLearning extracts a description from image data.
func ProcessImageForLearning(ctx context.Context, urlStr string, data []byte, mimeType string) (string, error) {
	if learnProviders.Image == nil {
		return "", fmt.Errorf("image processor not configured")
	}
	return learnProviders.Image.Describe(ctx, urlStr, data, mimeType)
}

// ProcessAudioForLearning transcribes audio content.
func ProcessAudioForLearning(ctx context.Context, urlStr string, data []byte, mimeType string) (string, error) {
	if learnProviders.Audio == nil {
		return "", fmt.Errorf("audio transcriber not configured")
	}
	return learnProviders.Audio.Transcribe(ctx, data, mimeType)
}

// ProcessURLForLearning processes a single URL and returns its content.
func ProcessURLForLearning(ctx context.Context, urlStr string) LearnContentResult {
	result := LearnContentResult{URL: urlStr}

	contentInfo, err := DetectContentTypeFromURL(ctx, urlStr)
	if err != nil && logger != nil {
		logger.Warnf("Failed to detect content type for %s, will determine after download", urlStr)
	}

	if contentInfo.Type == "video" {
		result.ContentType = "error"
		result.Error = fmt.Errorf("video content is not supported for learning")
		return result
	}

	if contentInfo.ContentLength > MaxFileSizeBytes {
		result.ContentType = "error"
		result.Error = fmt.Errorf("file size (%d MB) exceeds maximum allowed size (50 MB)",
			contentInfo.ContentLength/(1024*1024))
		return result
	}

	if learnProviders.Downloader == nil {
		result.ContentType = "error"
		result.Error = fmt.Errorf("content downloader not configured")
		return result
	}

	mimeType, data, err := learnProviders.Downloader.Download(ctx, urlStr)
	if err != nil {
		result.ContentType = "error"
		result.Error = fmt.Errorf("download failed: %w", err)
		return result
	}

	if int64(len(data)) > MaxFileSizeBytes {
		result.ContentType = "error"
		result.Error = fmt.Errorf("file size (%d MB) exceeds maximum allowed size (50 MB)",
			len(data)/(1024*1024))
		return result
	}

	contentType := contentInfo.Type
	if contentType == "unknown" || contentType == "" {
		if strings.Contains(mimeType, "pdf") {
			contentType = "pdf"
		} else if strings.HasPrefix(mimeType, "image/") {
			contentType = "image"
		} else if strings.HasPrefix(mimeType, "audio/") {
			contentType = "audio"
		} else if strings.HasPrefix(mimeType, "video/") {
			contentType = "video"
		} else if strings.HasPrefix(mimeType, "text/") {
			contentType = "text"
		} else {
			contentType = "unknown"
		}
	}

	if contentType == "video" {
		result.ContentType = "error"
		result.Error = fmt.Errorf("video content is not supported for learning")
		return result
	}

	result.ContentType = contentType

	switch contentType {
	case "pdf":
		content, err := ProcessPDFForLearning(ctx, urlStr, data)
		if err != nil {
			result.Error = err
			result.ContentType = "error"
			return result
		}
		result.Content = content

	case "image":
		content, err := ProcessImageForLearning(ctx, urlStr, data, mimeType)
		if err != nil {
			result.Error = err
			result.ContentType = "error"
			return result
		}
		result.Content = content

	case "audio":
		content, err := ProcessAudioForLearning(ctx, urlStr, data, mimeType)
		if err != nil {
			result.Error = err
			result.ContentType = "error"
			return result
		}
		result.Content = content

	case "text":
		result.Content = string(data)

	default:
		result.Content = string(data)
		result.ContentType = "text"
	}

	return result
}

// ProcessAllContent combines text and URL results into a single thread for learning.
func ProcessAllContent(ctx context.Context, plainText string, urlResults []LearnContentResult) (*thread.Thread, []string) {
	var errs []string
	var contentBuilder strings.Builder

	if strings.TrimSpace(plainText) != "" {
		contentBuilder.WriteString("## Text Content\n")
		contentBuilder.WriteString(plainText)
		contentBuilder.WriteString("\n\n")
	}

	for i, result := range urlResults {
		if result.Error != nil {
			errMsg := fmt.Sprintf("Failed to process URL %s: %v", result.URL, result.Error)
			if logger != nil {
				logger.Warnf(errMsg)
			}
			errs = append(errs, errMsg)
			continue
		}

		if result.Content == "" {
			errMsg := fmt.Sprintf("No content extracted from URL %s", result.URL)
			if logger != nil {
				logger.Warnf(errMsg)
			}
			errs = append(errs, errMsg)
			continue
		}

		contentBuilder.WriteString(fmt.Sprintf("## Content from URL #%d (%s)\n", i+1, result.ContentType))
		contentBuilder.WriteString(fmt.Sprintf("Source: %s\n\n", result.URL))
		contentBuilder.WriteString(result.Content)
		contentBuilder.WriteString("\n\n")
	}

	messageThread := thread.New()
	combinedContent := strings.TrimSpace(contentBuilder.String())

	if combinedContent == "" {
		messageThread.AddMessages(thread.NewUserMessage().AddContent(
			thread.NewTextContent("Unable to extract any content from the provided input.")))
	} else {
		messageThread.AddMessages(thread.NewUserMessage().AddContent(
			thread.NewTextContent(combinedContent)))
	}

	return messageThread, errs
}

// ProcessURLsInParallel processes multiple URLs concurrently and returns results in order.
func ProcessURLsInParallel(ctx context.Context, urls []string) []LearnContentResult {
	if len(urls) == 0 {
		return nil
	}

	results := make([]LearnContentResult, len(urls))
	var wg sync.WaitGroup

	for i, urlStr := range urls {
		wg.Add(1)
		go func(index int, u string) {
			defer wg.Done()
			results[index] = ProcessURLForLearning(ctx, u)
		}(i, urlStr)
	}

	wg.Wait()
	return results
}
