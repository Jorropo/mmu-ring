package ring

import (
	"bytes"
	"testing"
)

func TestNew(t *testing.T) {
	pageSize := uintptr(4096) // Assume 4KB page size for tests

	tests := []struct {
		name    string
		size    uintptr
		wantErr bool
	}{
		{"Valid size", pageSize, false},
		{"Invalid size", pageSize - 1, true},
		{"Zero size", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(tt.size)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && r == nil {
				t.Errorf("New() returned nil Ring for valid size")
			}
			if !tt.wantErr {
				if r.Size() != tt.size {
					t.Errorf("New() ring size = %v, want %v", r.Size(), tt.size)
				}
				r.Close()
			}
		})
	}
}

func TestRing_WriteAndRead(t *testing.T) {
	r, err := New(4096)
	if err != nil {
		t.Fatalf("Failed to create Ring: %v", err)
	}
	defer r.Close()

	testData := []byte("Hello, Ring Buffer!")

	// Test Write
	n, err := r.Write(func(buffer []byte) (uintptr, error) {
		return uintptr(copy(buffer, testData)), nil
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != uintptr(len(testData)) {
		t.Errorf("Write() wrote %d bytes, want %d", n, len(testData))
	}

	// Test Read
	readData := make([]byte, len(testData))
	n, err = r.Read(func(buffer []byte) (uintptr, error) {
		return uintptr(copy(readData, buffer)), nil
	})
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != uintptr(len(testData)) {
		t.Errorf("Read() read %d bytes, want %d", n, len(testData))
	}
	if !bytes.Equal(readData, testData) {
		t.Errorf("Read() = %q, want %q", readData, testData)
	}
}

func TestRing_UnusedAndContent(t *testing.T) {
	r, err := New(4096)
	if err != nil {
		t.Fatalf("Failed to create Ring: %v", err)
	}
	defer r.Close()

	initialUnused := len(r.Unused())
	if initialUnused != 4096 {
		t.Errorf("Initial Unused() = %d, want 4096", initialUnused)
	}

	testData := []byte("Test data")
	copy(r.Unused(), testData)
	r.Advance(uintptr(len(testData)))

	unusedAfterWrite := len(r.Unused())
	if unusedAfterWrite != 4096-len(testData) {
		t.Errorf("Unused() after write = %d, want %d", unusedAfterWrite, 4096-len(testData))
	}

	content := r.Content()
	if !bytes.Equal(content, testData) {
		t.Errorf("Content() = %q, want %q", content, testData)
	}
}

func TestRing_AdvanceAndConsume(t *testing.T) {
	r, err := New(4096)
	if err != nil {
		t.Fatalf("Failed to create Ring: %v", err)
	}
	defer r.Close()

	testData := []byte("Advance and Consume test")
	copy(r.Unused(), testData)

	err = r.Advance(uintptr(len(testData)))
	if err != nil {
		t.Errorf("Advance() failed: %v", err)
	}

	if r.contentLen != uintptr(len(testData)) {
		t.Errorf("usedSpace() = %d, want %d", r.contentLen, len(testData))
	}

	err = r.Consume(uintptr(len(testData)))
	if err != nil {
		t.Errorf("Consume() failed: %v", err)
	}

	if r.contentLen != 0 {
		t.Errorf("usedSpace() after Consume = %d, want 0", r.contentLen)
	}
}

func TestRing_Wraparound(t *testing.T) {
	pageSize := uintptr(4096)
	r, err := New(pageSize) // Use a full page size
	if err != nil {
		t.Fatalf("Failed to create Ring: %v", err)
	}
	defer r.Close()

	// Fill the buffer
	initialData := bytes.Repeat([]byte("abcdefghijklmnop"), 256) // 4096 bytes
	copy(r.Unused(), initialData)
	err = r.Advance(pageSize)
	if err != nil {
		t.Fatalf("Failed to advance: %v", err)
	}

	// Consume half
	err = r.Consume(pageSize / 2)
	if err != nil {
		t.Fatalf("Failed to consume: %v", err)
	}

	// Write more data to force wraparound
	newData := bytes.Repeat([]byte("ABCDEFGHIJKLMNOP"), 128) // 2048 bytes
	copy(r.Unused(), newData)
	err = r.Advance(pageSize / 2)
	if err != nil {
		t.Fatalf("Failed to advance: %v", err)
	}

	expected := append(initialData[pageSize/2:], newData...)
	content := r.Content()
	if !bytes.Equal(content, expected) {
		t.Errorf("Wraparound Content() doesn't match expected.")
		t.Errorf("Content length: %d, Expected length: %d", len(content), len(expected))
		t.Errorf("First 32 bytes of Content: %q", content[:32])
		t.Errorf("First 32 bytes of Expected: %q", expected[:32])
	}
}

func TestRing_ErrorCases(t *testing.T) {
	r, err := New(4096)
	if err != nil {
		t.Fatalf("Failed to create Ring: %v", err)
	}
	defer r.Close()

	// Test Advance error
	err = r.Advance(4097)
	if err == nil {
		t.Error("Advance() with size > buffer should return error")
	}

	// Test Consume error
	err = r.Consume(1)
	if err == nil {
		t.Error("Consume() with empty buffer should return error")
	}

	// Fill buffer
	copy(r.Unused(), bytes.Repeat([]byte("a"), 4096))
	r.Advance(4096)

	// Test Write error
	_, err = r.Write(func(buffer []byte) (uintptr, error) {
		return 1, nil // Try to write to full buffer
	})
	if err == nil {
		t.Error("Write() to full buffer should return error")
	}
}
