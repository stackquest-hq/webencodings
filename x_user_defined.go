package webencodings

import (
	"errors"
	"io"
	"unicode/utf8"
)

var (
	// ErrInvalidByte is returned when an invalid byte is encountered during encoding
	ErrInvalidByte = errors.New("webencodings: invalid byte in x-user-defined encoding")
	// ErrInvalidRune is returned when an invalid rune is encountered during decoding
	ErrInvalidRune = errors.New("webencodings: invalid rune in x-user-defined encoding")

	// EncodingTable provides reverse lookup from rune to byte for efficient encoding
	EncodingTable map[rune]byte
)

// init initializes the encoding table for efficient lookups
func init() {
	EncodingTable = make(map[rune]byte, 256)
	for i, r := range DecodingTable {
		EncodingTable[r] = byte(i)
	}
}

// XUserDefinedEncoder provides incremental encoding functionality
type XUserDefinedEncoder struct {
	pending []byte
	codec   *Codec
}

// NewXUserDefinedEncoder creates a new incremental encoder
func NewXUserDefinedEncoder() *XUserDefinedEncoder {
	return &XUserDefinedEncoder{
		codec: NewCodec(),
	}
}

// Encode incrementally encodes input and returns the encoded bytes
func (e *XUserDefinedEncoder) Encode(input []byte, final bool) ([]byte, error) {
	// Combine pending bytes with new input
	data := append(e.pending, input...)
	e.pending = nil

	if !final && len(data) > 0 {
		// Check if the last bytes form an incomplete UTF-8 sequence
		for i := len(data) - 1; i >= 0 && i >= len(data)-4; i-- {
			if utf8.RuneStart(data[i]) {
				if r, size := utf8.DecodeRune(data[i:]); r == utf8.RuneError && size == 1 {
					// Incomplete sequence, save for next call
					e.pending = make([]byte, len(data)-i)
					copy(e.pending, data[i:])
					data = data[:i]
				}
				break
			}
		}
	}

	// Convert to string and encode
	s := string(data)
	return e.codec.Encode(s, "strict")
}

// Reset resets the encoder state
func (e *XUserDefinedEncoder) Reset() {
	e.pending = nil
}

// XUserDefinedDecoder provides incremental decoding functionality
type XUserDefinedDecoder struct {
	codec *Codec
}

// NewXUserDefinedDecoder creates a new incremental decoder
func NewXUserDefinedDecoder() *XUserDefinedDecoder {
	return &XUserDefinedDecoder{
		codec: NewCodec(),
	}
}

// Decode incrementally decodes input and returns the decoded string
func (d *XUserDefinedDecoder) Decode(input []byte, final bool) (string, error) {
	return d.codec.Decode(input, "strict")
}

// Reset resets the decoder state
func (d *XUserDefinedDecoder) Reset() {
	// No state to reset for x-user-defined decoder
}

// Codec provides the main encoding/decoding functionality
type Codec struct{}

// NewCodec creates a new x-user-defined codec
func NewCodec() *Codec {
	return &Codec{}
}

// Encode encodes a string using the x-user-defined encoding
func (c *Codec) Encode(input string, errors string) ([]byte, error) {
	if errors != "strict" && errors != "ignore" && errors != "replace" {
		return nil, ErrInvalidByte
	}

	result := make([]byte, 0, len(input))

	for _, r := range input {
		if b, found := EncodingTable[r]; found {
			result = append(result, b)
		} else {
			if errors == "strict" {
				return nil, ErrInvalidRune
			} else if errors == "ignore" {
				continue
			} else if errors == "replace" {
				result = append(result, '?')
			}
		}
	}

	return result, nil
}

// Decode decodes bytes using the x-user-defined encoding
func (c *Codec) Decode(input []byte, errors string) (string, error) {
	if errors != "strict" && errors != "ignore" && errors != "replace" {
		return "", ErrInvalidByte
	}

	if len(input) == 0 {
		return "", nil
	}

	result := make([]rune, 0, len(input))

	for _, b := range input {
		result = append(result, DecodingTable[b])
	}

	return string(result), nil
}

// StreamWriter provides streaming write functionality
type StreamWriter struct {
	writer io.Writer
	codec  *Codec
}

// NewStreamWriter creates a new stream writer
func NewStreamWriter(w io.Writer) *StreamWriter {
	return &StreamWriter{
		writer: w,
		codec:  NewCodec(),
	}
}

// Write encodes and writes data to the underlying writer
func (sw *StreamWriter) Write(data []byte) (int, error) {
	encoded, err := sw.codec.Encode(string(data), "strict")
	if err != nil {
		return 0, err
	}

	n, err := sw.writer.Write(encoded)
	if err != nil {
		return 0, err
	}

	// Return the number of input bytes processed
	if n == len(encoded) {
		return len(data), nil
	}
	return 0, io.ErrShortWrite
}

// StreamReader provides streaming read functionality
type StreamReader struct {
	reader io.Reader
	codec  *Codec
}

// NewStreamReader creates a new stream reader
func NewStreamReader(r io.Reader) *StreamReader {
	return &StreamReader{
		reader: r,
		codec:  NewCodec(),
	}
}

// Read reads and decodes data from the underlying reader
func (sr *StreamReader) Read(buf []byte) (int, error) {
	// Read encoded data
	encoded := make([]byte, len(buf))
	n, err := sr.reader.Read(encoded)
	if err != nil && err != io.EOF {
		return 0, err
	}

	if n == 0 {
		return 0, err
	}

	// Decode the data
	decoded, decodeErr := sr.codec.Decode(encoded[:n], "strict")
	if decodeErr != nil {
		return 0, decodeErr
	}

	// Copy decoded data to buffer
	decodedBytes := []byte(decoded)
	copy(buf, decodedBytes)

	if len(decodedBytes) > len(buf) {
		return len(buf), nil
	}

	return len(decodedBytes), err
}

// CodecInfo holds information about the x-user-defined codec
type CodecInfo struct {
	Name               string
	Encode             func(string, string) ([]byte, error)
	Decode             func([]byte, string) (string, error)
	IncrementalEncoder func() *XUserDefinedEncoder
	IncrementalDecoder func() *XUserDefinedDecoder
	StreamReader       func(io.Reader) *StreamReader
	StreamWriter       func(io.Writer) *StreamWriter
}

// GetCodecInfo returns codec information for x-user-defined encoding
func GetCodecInfo() *CodecInfo {
	codec := NewCodec()
	return &CodecInfo{
		Name:   "x-user-defined",
		Encode: codec.Encode,
		Decode: codec.Decode,
		IncrementalEncoder: func() *XUserDefinedEncoder {
			return NewXUserDefinedEncoder()
		},
		IncrementalDecoder: func() *XUserDefinedDecoder {
			return NewXUserDefinedDecoder()
		},
		StreamReader: func(r io.Reader) *StreamReader {
			return NewStreamReader(r)
		},
		StreamWriter: func(w io.Writer) *StreamWriter {
			return NewStreamWriter(w)
		},
	}
}
