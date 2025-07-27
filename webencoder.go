package webencodings

import (
	"bytes"
	"errors"
	"strings"
)

const Version = "0.6-dev"

var (
	// ErrUnknownEncoding is returned when an unknown encoding label is encountered
	ErrUnknownEncoding = errors.New("webencodings: unknown encoding label")
	// ErrShortWrite is returned when not all data could be written
	ErrShortWrite = errors.New("webencodings: short write")
)

// PythonNames maps some encoding names that are not valid Python aliases
var PythonNames = map[string]string{
	"iso-8859-8-i":   "iso-8859-8",
	"x-mac-cyrillic": "mac-cyrillic",
	"macintosh":      "mac-roman",
	"windows-874":    "cp874",
}

// EncodingInfo represents a character encoding such as UTF-8
type EncodingInfo struct {
	// Name is the canonical name of the encoding
	Name string
	// CodecInfo is the actual implementation of the encoding
	CodecInfo interface{}
}

// String returns a string representation of the encoding
func (e *EncodingInfo) String() string {
	return "<Encoding " + e.Name + ">"
}

// Cache stores encoding objects to avoid repeated lookups
var Cache = make(map[string]*EncodingInfo)

// ASCIILower transforms (only) ASCII letters to lower case: A-Z is mapped to a-z.
// This is used for ASCII case-insensitive matching of encoding labels.
func ASCIILower(s string) string {
	var result []rune
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			result = append(result, r+32)
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

// Lookup looks for an encoding by its label.
// This implements the spec's "get an encoding" algorithm.
func Lookup(label string) *EncodingInfo {
	// Only strip ASCII whitespace: U+0009, U+000A, U+000C, U+000D, and U+0020.
	label = ASCIILower(strings.Trim(label, "\t\n\f\r "))

	name, exists := Labels[label]
	if !exists {
		return nil
	}

	encoding, exists := Cache[name]
	if !exists {
		var codecInfo interface{}

		if name == "x-user-defined" {
			codecInfo = GetCodecInfo()
		} else {
			// For other encodings, we create a basic info object without full codec support
			// This allows label lookup to work even if full encoding/decoding isn't implemented
			codecInfo = nil
		}

		encoding = &EncodingInfo{
			Name:      name,
			CodecInfo: codecInfo,
		}
		Cache[name] = encoding
	}

	return encoding
}

// getEncoding accepts either an encoding object or label and returns an EncodingInfo object
func getEncoding(encodingOrLabel interface{}) (*EncodingInfo, error) {
	if enc, ok := encodingOrLabel.(*EncodingInfo); ok {
		return enc, nil
	}

	if label, ok := encodingOrLabel.(string); ok {
		encoding := Lookup(label)
		if encoding == nil {
			return nil, ErrUnknownEncoding
		}
		return encoding, nil
	}

	return nil, ErrUnknownEncoding
}

// UTF8 is the UTF-8 encoding. Should be used for new content and formats.
var UTF8 *EncodingInfo

var (
	utf16LE *EncodingInfo
	utf16BE *EncodingInfo
)

// Initialize the encoding variables lazily to avoid circular dependencies
func init() {
	UTF8 = Lookup("utf-8")
	utf16LE = Lookup("utf-16le")
	utf16BE = Lookup("utf-16be")
}

// DetectBOM detects and removes BOM from input, returning the detected encoding and remaining data
func DetectBOM(input []byte) (*EncodingInfo, []byte) {
	if bytes.HasPrefix(input, []byte{0xFF, 0xFE}) {
		return utf16LE, input[2:]
	}
	if bytes.HasPrefix(input, []byte{0xFE, 0xFF}) {
		return utf16BE, input[2:]
	}
	if bytes.HasPrefix(input, []byte{0xEF, 0xBB, 0xBF}) {
		return UTF8, input[3:]
	}
	return nil, input
}

// Decode decodes a single byte string
func Decode(input []byte, fallbackEncoding interface{}, errors string) (string, *EncodingInfo, error) {
	if errors == "" {
		errors = "replace"
	}

	// Fail early if encoding is invalid
	fallbackEnc, err := getEncoding(fallbackEncoding)
	if err != nil {
		return "", nil, err
	}

	bomEncoding, remaining := DetectBOM(input)
	encoding := bomEncoding
	if encoding == nil {
		encoding = fallbackEnc
	}

	// For x-user-defined encoding
	if encoding.Name == "x-user-defined" {
		if codecInfo, ok := encoding.CodecInfo.(*CodecInfo); ok {
			decoded, err := codecInfo.Decode(remaining, errors)
			return decoded, encoding, err
		}
	}

	// For other encodings, we'd need to implement Go's encoding support
	return string(remaining), encoding, nil
}

// Encode encodes a single string
func Encode(input string, encoding interface{}, errors string) ([]byte, error) {
	if errors == "" {
		errors = "strict"
	}

	enc, err := getEncoding(encoding)
	if err != nil {
		return nil, err
	}

	// For x-user-defined encoding
	if enc.Name == "x-user-defined" {
		if codecInfo, ok := enc.CodecInfo.(*CodecInfo); ok {
			return codecInfo.Encode(input, errors)
		}
	}

	// For other encodings, we'd need to implement Go's encoding support
	return []byte(input), nil
}

// IncrementalDecoder provides "push"-based decoding
type IncrementalDecoder struct {
	fallbackEncoding *EncodingInfo
	errors           string
	buffer           []byte
	decoder          func([]byte, bool) (string, error)
	// Encoding is the actual encoding being used, or nil if not determined yet
	Encoding *EncodingInfo
}

// NewIncrementalDecoder creates a new incremental decoder
func NewIncrementalDecoder(fallbackEncoding interface{}, errors string) (*IncrementalDecoder, error) {
	if errors == "" {
		errors = "replace"
	}

	fallbackEnc, err := getEncoding(fallbackEncoding)
	if err != nil {
		return nil, err
	}

	return &IncrementalDecoder{
		fallbackEncoding: fallbackEnc,
		errors:           errors,
		buffer:           []byte{},
		decoder:          nil,
		Encoding:         nil,
	}, nil
}

// Decode decodes one chunk of input
func (d *IncrementalDecoder) Decode(input []byte, final bool) (string, error) {
	if d.decoder != nil {
		return d.decoder(input, final)
	}

	input = append(d.buffer, input...)
	encoding, remaining := DetectBOM(input)

	if encoding == nil {
		if len(input) < 3 && !final {
			// Not enough data yet
			d.buffer = input
			return "", nil
		} else {
			// No BOM
			encoding = d.fallbackEncoding
		}
	}

	// Set up decoder based on encoding
	if encoding.Name == "x-user-defined" {
		if _, ok := encoding.CodecInfo.(*CodecInfo); ok {
			decoder := NewXUserDefinedDecoder()
			d.decoder = func(data []byte, final bool) (string, error) {
				return decoder.Decode(data, final)
			}
		}
	}

	d.Encoding = encoding
	if d.decoder != nil {
		return d.decoder(remaining, final)
	}

	// Fallback for unsupported encodings
	return string(remaining), nil
}

// IncrementalEncoder provides "push"-based encoding
type IncrementalEncoder struct {
	encode func(string, bool) ([]byte, error)
}

// NewIncrementalEncoder creates a new incremental encoder
func NewIncrementalEncoder(encoding interface{}, errors string) (*IncrementalEncoder, error) {
	if errors == "" {
		errors = "strict"
	}

	enc, err := getEncoding(encoding)
	if err != nil {
		return nil, err
	}

	encoder := &IncrementalEncoder{}

	// Set up encoder based on encoding
	if enc.Name == "x-user-defined" {
		if _, ok := enc.CodecInfo.(*CodecInfo); ok {
			xuEncoder := NewXUserDefinedEncoder()
			encoder.encode = func(input string, final bool) ([]byte, error) {
				return xuEncoder.Encode([]byte(input), final)
			}
		}
	} else {
		// Fallback for unsupported encodings
		encoder.encode = func(input string, final bool) ([]byte, error) {
			return []byte(input), nil
		}
	}

	return encoder, nil
}

// Encode encodes input and returns the encoded bytes
func (e *IncrementalEncoder) Encode(input string, final bool) ([]byte, error) {
	if e.encode != nil {
		return e.encode(input, final)
	}
	return []byte(input), nil
}

// IterDecode provides "pull"-based decoding
func IterDecode(input <-chan []byte, fallbackEncoding interface{}, errors string) (<-chan string, *EncodingInfo, error) {
	if errors == "" {
		errors = "replace"
	}

	decoder, err := NewIncrementalDecoder(fallbackEncoding, errors)
	if err != nil {
		return nil, nil, err
	}

	output := make(chan string)

	go func() {
		defer close(output)

		for chunk := range input {
			decoded, err := decoder.Decode(chunk, false)
			if err != nil {
				return
			}
			if decoded != "" {
				output <- decoded
			}
		}

		// Final decode
		final, err := decoder.Decode([]byte{}, true)
		if err == nil && final != "" {
			output <- final
		}
	}()

	// Wait for first chunk to determine encoding
	firstChunk, ok := <-input
	if ok {
		decoded, err := decoder.Decode(firstChunk, false)
		if err != nil {
			return nil, nil, err
		}

		// Create new output channel that includes the first decoded chunk
		newOutput := make(chan string)
		go func() {
			defer close(newOutput)
			if decoded != "" {
				newOutput <- decoded
			}
			for chunk := range output {
				newOutput <- chunk
			}
		}()

		return newOutput, decoder.Encoding, nil
	}

	return output, decoder.Encoding, nil
}

// IterEncode provides "pull"-based encoding
func IterEncode(input <-chan string, encoding interface{}, errors string) (<-chan []byte, error) {
	if errors == "" {
		errors = "strict"
	}

	encoder, err := NewIncrementalEncoder(encoding, errors)
	if err != nil {
		return nil, err
	}

	output := make(chan []byte)

	go func() {
		defer close(output)

		for chunk := range input {
			encoded, err := encoder.Encode(chunk, false)
			if err != nil {
				return
			}
			if len(encoded) > 0 {
				output <- encoded
			}
		}

		// Final encode
		final, err := encoder.Encode("", true)
		if err == nil && len(final) > 0 {
			output <- final
		}
	}()

	return output, nil
}
