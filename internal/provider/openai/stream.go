package openai

import (
	"bytes"
	"encoding/json"
	"sync"
)

const (
	MaxEventBufferSize = 64 * 1024
)

// StreamParseResult holds the state, model metadata and usage metrics extracted from a stream
type StreamParseResult struct {
	ReportedModel  string
	ExtractedUsage *ExtractedUsage
	SawDone        bool
	OversizedEvent bool
	MalformedJSON  bool
}

// StreamParser reads a streaming OpenAI response line by line and extracts
// the model, token usage, and end marker, without storing the full stream
type StreamParser struct {
	mu             sync.Mutex
	buf            []byte
	currentData    []byte
	reportedModel  string
	extractedUsage *ExtractedUsage
	sawDone        bool
	oversized      bool
	malformed      bool
	discarding     bool // true when skipping an oversized event until the next event boundary
}

// NewStreamParser creates a fresh StreamParser with zero initial allocations.
func NewStreamParser() *StreamParser {
	return &StreamParser{
		buf:         make([]byte, 0, 1024),
		currentData: make([]byte, 0, 1024),
	}
}

// Feed passes in the next piece of data received from the upstream stream.
// Network chunks can split an event anywhere, so Feed remembers any partial
// event and completes it when the rest arrives.
// Memory limit: if a single event grows past MaxEventBufferSize (64KB), the extra
// bytes are dropped and parsing resumes at the next event, so memory can't grow without bound.
func (p *StreamParser) Feed(chunk []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(chunk) == 0 {
		return
	}

	// Bounded check: if appending would exceed MaxEventBufferSize, engage discarding mode
	if len(p.buf)+len(chunk) > MaxEventBufferSize {
		p.oversized = true
		p.discarding = true
		p.currentData = p.currentData[:0]

		// Search chunk for an event boundary to recover gracefully
		if idx := bytes.Index(chunk, []byte("\n\n")); idx >= 0 {
			p.discarding = false
			p.buf = append(p.buf[:0], chunk[idx+2:]...)
			return
		}
		if idx := bytes.Index(chunk, []byte("\n\r\n")); idx >= 0 {
			p.discarding = false
			p.buf = append(p.buf[:0], chunk[idx+3:]...)
			return
		}
		// No boundary found; discard buffer and wait for subsequent chunks
		p.buf = p.buf[:0]
		return
	}

	p.buf = append(p.buf, chunk...)

	// Scan buffer for completed lines
	for {
		nlIdx := bytes.IndexByte(p.buf, '\n')
		if nlIdx < 0 {
			break // Line incomplete await more bytes
		}

		line := p.buf[:nlIdx]
		p.buf = p.buf[nlIdx+1:]

		// Trim optional trailing \r for Windows CRLF line endings
		line = bytes.TrimSuffix(line, []byte("\r"))

		// An empty line marks an SSE event boundary
		if len(line) == 0 {
			if p.discarding {
				p.discarding = false
				p.currentData = p.currentData[:0]
				continue
			}
			if len(p.currentData) > 0 {
				p.processEvent(p.currentData)
				p.currentData = p.currentData[:0]
			}
			continue
		}

		if p.discarding {
			continue
		}

		// Parse SSE fields
		if bytes.HasPrefix(line, []byte("data:")) {
			data := bytes.TrimPrefix(line, []byte("data:"))
			data = bytes.TrimPrefix(data, []byte(" ")) 

			// Enforce memory bounds on accumulated event data
			if len(p.currentData)+len(data)+1 > MaxEventBufferSize {
				p.oversized = true
				p.discarding = true
				p.currentData = p.currentData[:0]
				continue
			}

			if len(p.currentData) > 0 {
				p.currentData = append(p.currentData, '\n')
			}
			p.currentData = append(p.currentData, data...)
		}
	}
}

// processEvent inspects an assembled SSE data payload.
func (p *StreamParser) processEvent(data []byte) {
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("[DONE]")) {
		p.sawDone = true
		return
	}

	var chunk struct {
		Model string    `json:"model"`
		Usage *rawUsage `json:"usage"`
	}

	if err := json.Unmarshal(trimmed, &chunk); err != nil {
		p.malformed = true
		return
	}

	if chunk.Model != "" && p.reportedModel == "" {
		p.reportedModel = chunk.Model
	}

	if chunk.Usage != nil {
		extracted, err := validateUsage(chunk.Model, chunk.Usage)
		if err != nil {
			p.malformed = true
		}
		p.extractedUsage = extracted
	}
}

// Finalize flushes any remaining buffered event data and returns the completed StreamParseResult.
func (p *StreamParser) Finalize() StreamParseResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	// If residual line remains in buf without a trailing newline, process it as a line
	if len(p.buf) > 0 && !p.discarding {
		line := bytes.TrimSuffix(p.buf, []byte("\r"))
		p.buf = p.buf[:0]
		if bytes.HasPrefix(line, []byte("data:")) {
			data := bytes.TrimPrefix(line, []byte("data:"))
			data = bytes.TrimPrefix(data, []byte(" "))
			if len(p.currentData) > 0 {
				p.currentData = append(p.currentData, '\n')
			}
			p.currentData = append(p.currentData, data...)
		}
	}

	// If residual data remains without an empty line delimiter, process it as an event
	if len(p.currentData) > 0 {
		p.processEvent(p.currentData)
		p.currentData = p.currentData[:0]
	}

	return StreamParseResult{
		ReportedModel:  p.reportedModel,
		ExtractedUsage: p.extractedUsage,
		SawDone:        p.sawDone,
		OversizedEvent: p.oversized,
		MalformedJSON:  p.malformed,
	}
}
