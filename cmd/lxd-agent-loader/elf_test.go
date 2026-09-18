package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func writeTempBinary(t *testing.T, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatalf("write temp binary: %v", err)
	}

	return path
}

func elf64Header(endian binary.ByteOrder, data byte, phoff uint64, phnum uint16) []byte {
	hdr := make([]byte, elfHeaderSize)
	copy(hdr, elfMagic)
	hdr[4] = elfClass64
	hdr[5] = data

	endian.PutUint64(hdr[32:40], phoff)
	endian.PutUint16(hdr[54:56], elfPhdrMinSize)
	endian.PutUint16(hdr[56:58], phnum)

	return hdr
}

func elf64Phdr(endian binary.ByteOrder, pType uint32, pOffset, pFilesz uint64) []byte {
	phdr := make([]byte, elfPhdrMinSize)
	endian.PutUint32(phdr[0:4], pType)
	endian.PutUint64(phdr[8:16], pOffset)
	endian.PutUint64(phdr[32:40], pFilesz)

	return phdr
}

func padTo(buf []byte, size uint64) []byte {
	for uint64(len(buf)) < size {
		buf = append(buf, 0)
	}

	return buf
}

func TestElfInterpreterDynamic(t *testing.T) {
	const interp = "/lib64/ld-linux-x86-64.so.2"

	endian := binary.LittleEndian
	interpOffset := uint64(elfHeaderSize + 2*elfPhdrMinSize)

	buf := elf64Header(endian, elfDataLSB, elfHeaderSize, 2)
	buf = append(buf, elf64Phdr(endian, 1, 0, 0)...) // PT_LOAD
	buf = append(buf, elf64Phdr(endian, ptInterp, interpOffset, uint64(len(interp)+1))...)
	buf = padTo(buf, interpOffset)
	buf = append(buf, interp...)
	buf = append(buf, 0)

	got, err := elfInterpreter(writeTempBinary(t, buf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != interp {
		t.Fatalf("expected interpreter %q, got %q", interp, got)
	}
}

func TestElfInterpreterStatic(t *testing.T) {
	endian := binary.LittleEndian

	buf := elf64Header(endian, elfDataLSB, elfHeaderSize, 1)
	buf = append(buf, elf64Phdr(endian, 1, 0, 0)...) // PT_LOAD only

	got, err := elfInterpreter(writeTempBinary(t, buf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "" {
		t.Fatalf("expected no interpreter, got %q", got)
	}
}

func TestElfInterpreterBigEndian(t *testing.T) {
	const interp = "/lib/ld-musl-aarch64.so.1"

	endian := binary.BigEndian
	interpOffset := uint64(elfHeaderSize + elfPhdrMinSize)

	buf := elf64Header(endian, elfDataMSB, elfHeaderSize, 1)
	buf = append(buf, elf64Phdr(endian, ptInterp, interpOffset, uint64(len(interp)+1))...)
	buf = padTo(buf, interpOffset)
	buf = append(buf, interp...)
	buf = append(buf, 0)

	got, err := elfInterpreter(writeTempBinary(t, buf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != interp {
		t.Fatalf("expected interpreter %q, got %q", interp, got)
	}
}

func TestElfInterpreterNotELF(t *testing.T) {
	got, err := elfInterpreter(writeTempBinary(t, []byte("this is not an elf image")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "" {
		t.Fatalf("expected no interpreter, got %q", got)
	}
}

func TestElfInterpreterTruncatedHeader(t *testing.T) {
	buf := make([]byte, 20)
	copy(buf, elfMagic)

	got, err := elfInterpreter(writeTempBinary(t, buf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "" {
		t.Fatalf("expected no interpreter, got %q", got)
	}
}

func TestElfInterpreterTruncatedPhdrTable(t *testing.T) {
	endian := binary.LittleEndian

	buf := elf64Header(endian, elfDataLSB, elfHeaderSize, 4) // claims 4 entries
	buf = append(buf, elf64Phdr(endian, 1, 0, 0)...)
	buf = append(buf, elf64Phdr(endian, 1, 0, 0)...) // only 2 present

	got, err := elfInterpreter(writeTempBinary(t, buf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "" {
		t.Fatalf("expected no interpreter, got %q", got)
	}
}
