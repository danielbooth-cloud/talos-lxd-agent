package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	elfMagic   = "\x7fELF"
	elfClass64 = 2
	elfDataLSB = 1
	elfDataMSB = 2

	ptInterp = 3

	elfHeaderSize  = 64
	elfPhdrMinSize = 56 // sizeof(Elf64_Phdr)

	// Sanity limits so a corrupt header cannot make the loader read an
	// unbounded amount of data.
	maxPhdrTableSize  = 1 << 20
	maxPhdrEntrySize  = 4096
	maxInterpPathSize = 4096
)

// elfInterpreter returns the dynamic loader path (PT_INTERP) of the ELF64
// binary at path, or "" if the binary is statically linked (or not an ELF64
// image that can be inspected).
func elfInterpreter(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}

	defer f.Close() //nolint:errcheck

	return elfInterpreterFrom(f)
}

// elfInterpreterFrom inspects an ELF64 image read from r and returns its
// PT_INTERP loader path, or "" for a statically linked binary.
func elfInterpreterFrom(r io.ReaderAt) (string, error) {
	var hdr [elfHeaderSize]byte
	if _, err := r.ReadAt(hdr[:], 0); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			// Too small to be an ELF image.
			return "", nil
		}

		return "", err
	}

	if !bytes.HasPrefix(hdr[:], []byte(elfMagic)) || hdr[4] != elfClass64 {
		return "", nil
	}

	var endian binary.ByteOrder = binary.LittleEndian

	switch hdr[5] {
	case elfDataLSB:
		endian = binary.LittleEndian
	case elfDataMSB:
		endian = binary.BigEndian
	default:
		return "", nil
	}

	phoff := endian.Uint64(hdr[32:40])
	phentsize := int(endian.Uint16(hdr[54:56]))
	phnum := int(endian.Uint16(hdr[56:58]))

	if phoff == 0 || phnum == 0 ||
		phentsize < elfPhdrMinSize || phentsize > maxPhdrEntrySize ||
		phnum*phentsize > maxPhdrTableSize {
		return "", nil
	}

	table := make([]byte, phnum*phentsize)
	if _, err := r.ReadAt(table, int64(phoff)); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			// Truncated program header table.
			return "", nil
		}

		return "", err
	}

	for i := range phnum {
		phdr := table[i*phentsize : i*phentsize+elfPhdrMinSize]

		if endian.Uint32(phdr[0:4]) != ptInterp {
			continue
		}

		pOffset := endian.Uint64(phdr[8:16])
		pFilesz := endian.Uint64(phdr[32:40])

		if pFilesz == 0 || pFilesz > maxInterpPathSize {
			return "", fmt.Errorf("interpreter path size %d out of range", pFilesz)
		}

		raw := make([]byte, pFilesz)
		if _, err := r.ReadAt(raw, int64(pOffset)); err != nil {
			return "", fmt.Errorf("read interpreter path: %w", err)
		}

		return string(bytes.TrimRight(raw, "\x00")), nil
	}

	return "", nil
}
