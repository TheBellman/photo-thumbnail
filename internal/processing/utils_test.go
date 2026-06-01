package processing

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/TheBellman/photo-thumbnail/internal/storage"
)

// minJPEG is the smallest byte sequence that passes validateJPEG: SOI + EOI.
var minJPEG = []byte{0xFF, 0xD8, 0xFF, 0xD9}

func TestIsOrf(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "cr3",
			key:  "test.CR3",
			want: false,
		},
		{
			name: "orf",
			key:  "test.ORF",
			want: true,
		},
		{
			name: "jpeg",
			key:  "test.jpeg",
			want: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isORF(testFile(tt.key))
			if result != tt.want {
				t.Fatalf("isCR3() = %t, want %t", result, tt.want)
			}
		})
	}
}

func TestIsCr3(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "cr3",
			key:  "test.CR3",
			want: true,
		},
		{
			name: "orf",
			key:  "test.ORF",
			want: false,
		},
		{
			name: "jpeg",
			key:  "test.jpeg",
			want: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isCR3(testFile(tt.key))
			if result != tt.want {
				t.Fatalf("isCR3() = %t, want %t", result, tt.want)
			}
		})
	}
}

func TestReadImageBytes(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		contentType string
	}{
		{
			name:        "cr3",
			key:         "test.CR3",
			contentType: "image/x-canon-cr3",
		},
		{
			name:        "orf",
			key:         "test.ORF",
			contentType: "image/x-olympus-orf",
		},
		{
			name:        "jpeg",
			key:         "test.jpeg",
			contentType: "image/jpeg",
		},
		{
			name:        "heic",
			key:         "test.HEIC",
			contentType: "image/heic",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := readImageBytes(mustOpenFile(tt.key), tt.contentType)
			if err != nil {
				t.Fatalf("readImageBytes() error: %v", err)
			}
			if len(result) == 0 {
				t.Fatalf("readImageBytes() = empty")
			}
			if result == nil {
				t.Fatalf("readImageBytes() = returned nil")
			}
		})
	}
}

func TestReadImageBytesJPEG(t *testing.T) {
	t.Parallel()

	input := []byte{1, 2, 3, 4}
	got, err := readImageBytes(bytes.NewReader(input), storage.JPEG)
	if err != nil {
		t.Fatalf("readImageBytes() error: %v", err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("readImageBytes() = %v, want %v", got, input)
	}
}

func TestReadImageBytesReadError(t *testing.T) {
	t.Parallel()

	_, err := readImageBytes(errReader{}, storage.JPEG)
	if err == nil {
		t.Fatalf("expected error")
	}
}

// -----------------------------------------------------------------------
// validateJPEG
// -----------------------------------------------------------------------

func TestValidateJPEG(t *testing.T) {
	// A slightly longer JPEG with an APP0 marker between SOI and EOI.
	validLonger := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0xFF, 0xD9}

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "valid minimal JPEG (SOI+EOI only)",
			data:    minJPEG,
			wantErr: false,
		},
		{
			name:    "valid JPEG with content between markers",
			data:    validLonger,
			wantErr: false,
		},
		{
			name:    "nil slice",
			data:    nil,
			wantErr: true,
		},
		{
			name:    "empty slice",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "three bytes — too short",
			data:    []byte{0xFF, 0xD8, 0xFF},
			wantErr: true,
		},
		{
			name:    "wrong SOI first byte",
			data:    []byte{0x00, 0xD8, 0xFF, 0xD9},
			wantErr: true,
		},
		{
			name:    "wrong SOI second byte",
			data:    []byte{0xFF, 0x00, 0xFF, 0xD9},
			wantErr: true,
		},
		{
			name:    "wrong EOI first byte",
			data:    []byte{0xFF, 0xD8, 0x00, 0xD9},
			wantErr: true,
		},
		{
			name:    "wrong EOI second byte",
			data:    []byte{0xFF, 0xD8, 0xFF, 0x00},
			wantErr: true,
		},
		{
			name:    "SOI only, no EOI",
			data:    []byte{0xFF, 0xD8},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateJPEG(tc.data)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateJPEG() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// -----------------------------------------------------------------------
// jpegFromPRVW
// -----------------------------------------------------------------------

func TestJpegFromPRVW(t *testing.T) {
	// Realistic Canon PRVW payload prefix as seen on EOS 90D:
	//   4 bytes  version+flags  (00 00 00 00)
	//   2 bytes  width          (06 54 = 1620)
	//   2 bytes  height         (04 38 = 1080)
	//   2 bytes  unknown        (00 01)
	//   4 bytes  JPEG size      (filled in below)
	canonHeader := func(jpegLen int) []byte {
		hdr := []byte{
			0x00, 0x00, 0x00, 0x00, // version + flags
			0x06, 0x54, // width  1620
			0x04, 0x38, // height 1080
			0x00, 0x01, // unknown
		}
		sz := make([]byte, 4)
		binary.BigEndian.PutUint32(sz, uint32(jpegLen))
		return append(hdr, sz...)
	}

	tests := []struct {
		name     string
		data     []byte
		wantJPEG []byte
		wantErr  bool
	}{
		{
			name:     "JPEG starts at offset 0 — no header",
			data:     minJPEG,
			wantJPEG: minJPEG,
			wantErr:  false,
		},
		{
			name:     "realistic Canon 90D header before JPEG",
			data:     append(canonHeader(len(minJPEG)), minJPEG...),
			wantJPEG: minJPEG,
			wantErr:  false,
		},
		{
			name:     "arbitrary non-zero bytes before SOI",
			data:     append([]byte{0x00, 0x01, 0x02, 0x03}, minJPEG...),
			wantJPEG: minJPEG,
			wantErr:  false,
		},
		{
			name: "partial FF bytes before SOI do not confuse scanner",
			// 0xFF 0x00 is not a JPEG SOI, scanner must not stop there
			data:     append([]byte{0xFF, 0x00, 0xFF, 0x00}, minJPEG...),
			wantJPEG: minJPEG,
			wantErr:  false,
		},
		{
			name:    "nil data",
			data:    nil,
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "no SOI marker anywhere",
			data:    []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
			wantErr: true,
		},
		{
			name: "SOI found but data ends before EOI",
			// FF D8 FF present but no FF D9 at the end
			data:    []byte{0x00, 0x00, 0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x01},
			wantErr: true,
		},
		{
			name:    "SOI found but EOI bytes wrong",
			data:    []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0xD9}, // last byte 0xD9 but preceded by 0x00 not 0xFF
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := jpegFromPRVW(tc.data)
			if (err != nil) != tc.wantErr {
				t.Errorf("jpegFromPRVW() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !tc.wantErr && !bytes.Equal(got, tc.wantJPEG) {
				t.Errorf("jpegFromPRVW() = %x, want %x", got, tc.wantJPEG)
			}
		})
	}
}

// -----------------------------------------------------------------------
// jpegFromCMT1
// -----------------------------------------------------------------------

// buildTIFFWithThumb constructs a minimal valid little-endian TIFF that
// goexif can parse, with thumbJPEG embedded in IFD1.
//
// Layout:
//
//	Offset  0: TIFF header         (8 bytes)
//	Offset  8: IFD0                (2 + 12×1 + 4 = 18 bytes, 1 entry, next→IFD1)
//	Offset 26: IFD1                (2 + 12×2 + 4 = 30 bytes, 2 entries, next→0)
//	Offset 56: JPEG thumbnail data
func buildTIFFWithThumb(thumbJPEG []byte) []byte {
	const (
		ifd0At  = 8
		ifd1At  = 26
		thumbAt = 56
	)
	le := binary.LittleEndian
	buf := make([]byte, thumbAt+len(thumbJPEG))

	// TIFF header
	buf[0], buf[1] = 0x49, 0x49   // "II" little-endian
	le.PutUint16(buf[2:], 42)     // TIFF magic
	le.PutUint32(buf[4:], ifd0At) // IFD0 offset

	// IFD0: 1 entry (ImageWidth=1), next IFD → IFD1
	le.PutUint16(buf[8:], 1)       // entry count
	le.PutUint16(buf[10:], 0x0100) // tag: ImageWidth
	le.PutUint16(buf[12:], 3)      // type: SHORT
	le.PutUint32(buf[14:], 1)      // count
	le.PutUint32(buf[18:], 1)      // value: width = 1
	le.PutUint32(buf[22:], ifd1At) // next IFD pointer → IFD1

	// IFD1: 2 entries (JPEGInterchangeFormat + JPEGInterchangeFormatLength)
	// Tags must be in ascending order per TIFF spec.
	le.PutUint16(buf[26:], 2) // entry count

	// Tag 0x0201: JPEGInterchangeFormat — offset to thumbnail bytes
	le.PutUint16(buf[28:], 0x0201)
	le.PutUint16(buf[30:], 4)       // type: LONG
	le.PutUint32(buf[32:], 1)       // count
	le.PutUint32(buf[36:], thumbAt) // value: offset

	// Tag 0x0202: JPEGInterchangeFormatLength — byte length of thumbnail
	le.PutUint16(buf[40:], 0x0202)
	le.PutUint16(buf[42:], 4)                      // type: LONG
	le.PutUint32(buf[44:], 1)                      // count
	le.PutUint32(buf[48:], uint32(len(thumbJPEG))) // value: length

	le.PutUint32(buf[52:], 0) // IFD1 next IFD = 0 (none)

	copy(buf[thumbAt:], thumbJPEG)
	return buf
}

// buildTIFFNoIFD1 constructs a valid TIFF with IFD0 only and no IFD1 (no thumbnail).
func buildTIFFNoIFD1() []byte {
	le := binary.LittleEndian
	buf := make([]byte, 26) // header(8) + IFD0(2+12+4=18)

	buf[0], buf[1] = 0x49, 0x49
	le.PutUint16(buf[2:], 42)
	le.PutUint32(buf[4:], 8) // IFD0 at offset 8

	le.PutUint16(buf[8:], 1)       // 1 entry
	le.PutUint16(buf[10:], 0x0100) // tag: ImageWidth
	le.PutUint16(buf[12:], 3)      // type: SHORT
	le.PutUint32(buf[14:], 1)      // count
	le.PutUint32(buf[18:], 1)      // value: width = 1
	le.PutUint32(buf[22:], 0)      // next IFD = 0 (no IFD1)

	return buf
}

func TestJpegFromCMT1(t *testing.T) {
	t.Run("nil data returns error", func(t *testing.T) {
		_, err := jpegFromCMT1(nil)
		if err == nil {
			t.Error("expected error for nil data, got nil")
		}
	})

	t.Run("empty data returns error", func(t *testing.T) {
		_, err := jpegFromCMT1([]byte{})
		if err == nil {
			t.Error("expected error for empty data, got nil")
		}
	})

	t.Run("garbage data returns error", func(t *testing.T) {
		_, err := jpegFromCMT1([]byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x01, 0x02, 0x03})
		if err == nil {
			t.Error("expected error for garbage data, got nil")
		}
	})

	t.Run("valid TIFF without IFD1 returns error", func(t *testing.T) {
		tiff := buildTIFFNoIFD1()
		_, err := jpegFromCMT1(tiff)
		if err == nil {
			t.Error("expected error when no IFD1 thumbnail present, got nil")
		}
	})

	t.Run("valid TIFF with IFD1 thumbnail returns JPEG", func(t *testing.T) {
		tiff := buildTIFFWithThumb(minJPEG)
		got, err := jpegFromCMT1(tiff)
		if err != nil {
			t.Fatalf("jpegFromCMT1() unexpected error: %v", err)
		}
		if !bytes.Equal(got, minJPEG) {
			t.Errorf("jpegFromCMT1() = %x, want %x", got, minJPEG)
		}
	})

	t.Run("thumbnail passes validateJPEG", func(t *testing.T) {
		tiff := buildTIFFWithThumb(minJPEG)
		got, err := jpegFromCMT1(tiff)
		if err != nil {
			t.Fatalf("jpegFromCMT1() unexpected error: %v", err)
		}
		if err := validateJPEG(got); err != nil {
			t.Errorf("returned thumbnail failed validateJPEG: %v", err)
		}
	})
}

// -----------------------------------------------------------------------
// Test helpers — ISOBMFF box construction
// -----------------------------------------------------------------------

// box builds a standard 32-bit-size ISOBMFF box.
// size field = 8 (header) + len(payload).
func box(boxType string, payload []byte) []byte {
	size := uint32(8 + len(payload))
	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[0:], size)
	copy(buf[4:8], boxType)
	copy(buf[8:], payload)
	return buf
}

// largesizeBox builds an ISOBMFF box that uses a 64-bit largesize field
// (initial 32-bit size field = 1).
func largesizeBox(boxType string, payload []byte) []byte {
	size := uint64(16 + len(payload)) // 8 (size+type) + 8 (largesize) + payload
	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[0:], 1) // size == 1 → largesize follows
	copy(buf[4:8], boxType)
	binary.BigEndian.PutUint64(buf[8:], size)
	copy(buf[16:], payload)
	return buf
}

// eofBox builds an ISOBMFF box with size == 0, meaning "extends to EOF".
func eofBox(boxType string, payload []byte) []byte {
	buf := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(buf[0:], 0) // size == 0 → EOF-extent
	copy(buf[4:8], boxType)
	copy(buf[8:], payload)
	return buf
}

// cat concatenates byte slices — keeps test table rows readable.
func cat(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

// -----------------------------------------------------------------------
// parseBoxAt
// -----------------------------------------------------------------------

func TestParseBoxAt(t *testing.T) {
	payload4 := []byte{0x01, 0x02, 0x03, 0x04}
	payload0 := []byte{}

	t.Run("standard box at offset 0", func(t *testing.T) {
		data := box("moov", payload4)
		b, next, err := parseBoxAt(data, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.boxType != "moov" {
			t.Errorf("boxType = %q, want %q", b.boxType, "moov")
		}
		if !bytes.Equal(b.payload, payload4) {
			t.Errorf("payload = %x, want %x", b.payload, payload4)
		}
		if next != len(data) {
			t.Errorf("next = %d, want %d", next, len(data))
		}
	})

	t.Run("standard box at non-zero offset", func(t *testing.T) {
		prefix := box("ftyp", []byte("crx "))
		suffix := box("moov", payload4)
		data := cat(prefix, suffix)
		b, next, err := parseBoxAt(data, len(prefix))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.boxType != "moov" {
			t.Errorf("boxType = %q, want %q", b.boxType, "moov")
		}
		if !bytes.Equal(b.payload, payload4) {
			t.Errorf("payload = %x, want %x", b.payload, payload4)
		}
		if next != len(data) {
			t.Errorf("next = %d, want %d", next, len(data))
		}
	})

	t.Run("box with empty payload", func(t *testing.T) {
		data := box("free", payload0)
		b, next, err := parseBoxAt(data, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(b.payload) != 0 {
			t.Errorf("expected empty payload, got %x", b.payload)
		}
		if next != 8 {
			t.Errorf("next = %d, want 8", next)
		}
	})

	t.Run("largesize box (size field == 1)", func(t *testing.T) {
		data := largesizeBox("mdat", payload4)
		b, next, err := parseBoxAt(data, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.boxType != "mdat" {
			t.Errorf("boxType = %q, want %q", b.boxType, "mdat")
		}
		if !bytes.Equal(b.payload, payload4) {
			t.Errorf("payload = %x, want %x", b.payload, payload4)
		}
		if next != len(data) {
			t.Errorf("next = %d, want %d", next, len(data))
		}
	})

	t.Run("EOF-extent box (size field == 0)", func(t *testing.T) {
		data := eofBox("mdat", payload4)
		b, next, err := parseBoxAt(data, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.boxType != "mdat" {
			t.Errorf("boxType = %q, want %q", b.boxType, "mdat")
		}
		if !bytes.Equal(b.payload, payload4) {
			t.Errorf("payload = %x, want %x", b.payload, payload4)
		}
		if next != len(data) {
			t.Errorf("next = %d, want %d (EOF-extent should consume all remaining data)", next, len(data))
		}
	})

	t.Run("header truncated — fewer than 8 bytes available", func(t *testing.T) {
		data := box("moov", payload4)
		_, _, err := parseBoxAt(data[:7], 0) // only 7 bytes — header needs 8
		if err == nil {
			t.Error("expected error for truncated header, got nil")
		}
	})

	t.Run("header truncated at non-zero offset", func(t *testing.T) {
		prefix := box("ftyp", []byte("crx "))
		// append only 3 bytes of a second box (need at least 8)
		data := cat(prefix, []byte{0x00, 0x00, 0x00})
		_, _, err := parseBoxAt(data, len(prefix))
		if err == nil {
			t.Error("expected error for truncated header at non-zero offset, got nil")
		}
	})

	t.Run("largesize header truncated — fewer than 16 bytes available", func(t *testing.T) {
		data := largesizeBox("mdat", payload4)
		_, _, err := parseBoxAt(data[:15], 0) // need 16 for largesize header
		if err == nil {
			t.Error("expected error for truncated largesize header, got nil")
		}
	})

	t.Run("size smaller than minimum header length", func(t *testing.T) {
		// Manually craft a box where the 32-bit size field is 4 (< 8).
		data := []byte{0x00, 0x00, 0x00, 0x04, 't', 'e', 's', 't'}
		_, _, err := parseBoxAt(data, 0)
		if err == nil {
			t.Error("expected error for size < headerLen, got nil")
		}
	})

	t.Run("size exceeds available data", func(t *testing.T) {
		data := box("moov", payload4)
		// Inflate the declared size to be larger than the actual buffer.
		binary.BigEndian.PutUint32(data[0:], uint32(len(data)+100))
		_, _, err := parseBoxAt(data, 0)
		if err == nil {
			t.Error("expected error when declared size exceeds data length, got nil")
		}
	})

	t.Run("next offset is offset+size, not just size", func(t *testing.T) {
		// Two boxes; parse the second one and verify next points past it.
		first := box("ftyp", []byte("crx "))
		second := box("moov", payload4)
		data := cat(first, second)
		_, next, err := parseBoxAt(data, len(first))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if next != len(data) {
			t.Errorf("next = %d, want %d", next, len(data))
		}
	})
}

// -----------------------------------------------------------------------
// findBox
// -----------------------------------------------------------------------

func TestFindBox(t *testing.T) {
	t.Run("finds box at start of data", func(t *testing.T) {
		data := box("moov", []byte("content"))
		b, ok := findBox(data, "moov")
		if !ok {
			t.Fatal("expected to find 'moov', got not-found")
		}
		if b.boxType != "moov" {
			t.Errorf("boxType = %q, want %q", b.boxType, "moov")
		}
		if !bytes.Equal(b.payload, []byte("content")) {
			t.Errorf("payload = %q, want %q", b.payload, "content")
		}
	})

	t.Run("finds box after other boxes", func(t *testing.T) {
		data := cat(
			box("ftyp", []byte("crx ")),
			box("free", []byte{}),
			box("moov", []byte("moovdata")),
		)
		b, ok := findBox(data, "moov")
		if !ok {
			t.Fatal("expected to find 'moov', got not-found")
		}
		if !bytes.Equal(b.payload, []byte("moovdata")) {
			t.Errorf("payload = %q, want %q", b.payload, "moovdata")
		}
	})

	t.Run("returns first match when target appears multiple times", func(t *testing.T) {
		data := cat(
			box("uuid", []byte("first")),
			box("uuid", []byte("second")),
		)
		b, ok := findBox(data, "uuid")
		if !ok {
			t.Fatal("expected to find 'uuid', got not-found")
		}
		if !bytes.Equal(b.payload, []byte("first")) {
			t.Errorf("payload = %q, want %q (should return first match)", b.payload, "first")
		}
	})

	t.Run("returns not-found when target absent", func(t *testing.T) {
		data := cat(
			box("ftyp", []byte("crx ")),
			box("free", []byte{}),
		)
		_, ok := findBox(data, "moov")
		if ok {
			t.Error("expected not-found for absent target, got found")
		}
	})

	t.Run("returns not-found for empty data", func(t *testing.T) {
		_, ok := findBox([]byte{}, "moov")
		if ok {
			t.Error("expected not-found for empty data, got found")
		}
	})

	t.Run("returns not-found for nil data", func(t *testing.T) {
		_, ok := findBox(nil, "moov")
		if ok {
			t.Error("expected not-found for nil data, got found")
		}
	})

	t.Run("stops at parse error and returns not-found", func(t *testing.T) {
		// A corrupt box (size too small) followed by valid target — target must not be found.
		corrupt := []byte{0x00, 0x00, 0x00, 0x04, 'm', 'o', 'o', 'v'} // size=4 < 8
		target := box("moov", []byte("after-corrupt"))
		data := cat(corrupt, target)
		_, ok := findBox(data, "moov")
		if ok {
			t.Error("expected not-found: parser should stop at corrupt box, not skip over it")
		}
	})
}

// -----------------------------------------------------------------------
// findBoxes
// -----------------------------------------------------------------------

func TestFindBoxes(t *testing.T) {
	t.Run("returns empty slice for nil data", func(t *testing.T) {
		result := findBoxes(nil, "uuid")
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %d boxes", len(result))
		}
	})

	t.Run("returns empty slice when target absent", func(t *testing.T) {
		data := cat(
			box("ftyp", []byte("crx ")),
			box("moov", []byte("data")),
		)
		result := findBoxes(data, "uuid")
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %d boxes", len(result))
		}
	})

	t.Run("returns single match", func(t *testing.T) {
		data := cat(
			box("ftyp", []byte("crx ")),
			box("uuid", []byte("only")),
			box("mdat", []byte{}),
		)
		result := findBoxes(data, "uuid")
		if len(result) != 1 {
			t.Fatalf("expected 1 box, got %d", len(result))
		}
		if !bytes.Equal(result[0].payload, []byte("only")) {
			t.Errorf("payload = %q, want %q", result[0].payload, "only")
		}
	})

	t.Run("returns all matches in order", func(t *testing.T) {
		data := cat(
			box("uuid", []byte("alpha")),
			box("free", []byte{}),
			box("uuid", []byte("beta")),
			box("moov", []byte("ignored")),
			box("uuid", []byte("gamma")),
		)
		result := findBoxes(data, "uuid")
		if len(result) != 3 {
			t.Fatalf("expected 3 boxes, got %d", len(result))
		}
		want := []string{"alpha", "beta", "gamma"}
		for i, w := range want {
			if !bytes.Equal(result[i].payload, []byte(w)) {
				t.Errorf("result[%d].payload = %q, want %q", i, result[i].payload, w)
			}
		}
	})

	t.Run("does not return non-matching boxes", func(t *testing.T) {
		data := cat(
			box("ftyp", []byte("crx ")),
			box("uuid", []byte("wanted")),
			box("moov", []byte("unwanted")),
		)
		result := findBoxes(data, "uuid")
		for _, b := range result {
			if b.boxType != "uuid" {
				t.Errorf("unexpected box type %q in results", b.boxType)
			}
		}
	})

	t.Run("stops collecting at parse error", func(t *testing.T) {
		// Two valid uuid boxes, then a corrupt box, then another uuid.
		// Only the first two should be returned.
		corrupt := []byte{0x00, 0x00, 0x00, 0x04, 'u', 'u', 'i', 'd'} // size=4 < 8
		data := cat(
			box("uuid", []byte("first")),
			box("uuid", []byte("second")),
			corrupt,
			box("uuid", []byte("unreachable")),
		)
		result := findBoxes(data, "uuid")
		if len(result) != 2 {
			t.Errorf("expected 2 boxes (before corrupt), got %d", len(result))
		}
	})
}
