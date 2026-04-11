package hamr

// Content represents an MCP content block returned by tools.
// The Type field controls which other fields are meaningful:
// "text" uses Text, "image" uses MimeType and Data, "resource" uses URI, MimeType, and Text.
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"` // base64-encoded for image content
	URI      string `json:"uri,omitempty"`  // resource URI for resource content
}

// TextContent creates a plain-text content block.
func TextContent(text string) Content {
	return Content{
		Type: "text",
		Text: text,
	}
}

// ImageContent creates an image content block. data must be base64-encoded.
func ImageContent(mimeType, base64Data string) Content {
	return Content{
		Type:     "image",
		MimeType: mimeType,
		Data:     base64Data,
	}
}

// ResourceContent creates a resource content block referencing an external URI.
func ResourceContent(uri, mimeType, text string) Content {
	return Content{
		Type:     "resource",
		URI:      uri,
		MimeType: mimeType,
		Text:     text,
	}
}

// Result is the return type for tool handlers that produce one or more content blocks.
// Set IsError to true to signal that the tool call failed; the Content blocks should
// then carry the error description.
type Result struct {
	Content []Content
	IsError bool
}

// NewResult constructs a successful Result from the provided content blocks.
func NewResult(content ...Content) Result {
	return Result{Content: content}
}

// ErrorResult constructs an error Result containing a single text block with the
// supplied message. The IsError flag is set so the MCP client can distinguish
// tool errors from successful responses.
func ErrorResult(text string) Result {
	return Result{
		Content: []Content{TextContent(text)},
		IsError: true,
	}
}
