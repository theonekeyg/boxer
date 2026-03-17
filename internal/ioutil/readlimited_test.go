package ioutil

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadLimited_StopsAtLimit(t *testing.T) {
	data := []byte("hello world this is a longer string")
	r := bytes.NewReader(data)
	result, err := ReadLimited(r, 5)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
	if len(result) != 5 {
		t.Errorf("expected length 5, got %d", len(result))
	}
}

func TestReadLimited_ReadsAllIfUnderLimit(t *testing.T) {
	r := strings.NewReader("hi")
	result, err := ReadLimited(r, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "hi" {
		t.Errorf("expected 'hi', got %q", result)
	}
}

func TestReadLimited_EmptyInput(t *testing.T) {
	r := strings.NewReader("")
	result, err := ReadLimited(r, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty, got %q", result)
	}
}

func TestReadLimited_ExactlyLimit(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 100)
	r := bytes.NewReader(data)
	result, err := ReadLimited(r, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 100 {
		t.Errorf("expected 100, got %d", len(result))
	}
}

func TestReadLimited_ZeroLimit(t *testing.T) {
	r := strings.NewReader("data")
	result, err := ReadLimited(r, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty for zero limit, got %d bytes", len(result))
	}
}
