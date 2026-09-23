package brand

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	encodingUTF8  = 1
	encodingUTF16 = 2

	oldSuffix = " - Chromium"
	newSuffix = " - SafeRuBro"
)

// rewrite replaces Chromium's window-title suffix inside a locale pak.
// Chromium stores titles as "$1 - Chromium" and has no switch for that name.
// The returned bytes are unchanged when the suffix is already gone.
func rewrite(data []byte) ([]byte, int, error) {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		plain, err := gunzip(data)
		if err != nil {
			return nil, 0, err
		}
		plain, n, err := rewritePak(plain)
		if err != nil || n == 0 {
			return data, n, err
		}
		packed, err := gzipBytes(plain)
		if err != nil {
			return nil, 0, err
		}
		return packed, n, nil
	}
	return rewritePak(data)
}

func rewritePak(data []byte) ([]byte, int, error) {
	pak, err := parsePak(data)
	if err != nil {
		return nil, 0, err
	}
	from, to := suffixPair(pak.encoding)
	if from == nil {
		return data, 0, nil
	}

	replaced := 0
	blobs := make([][]byte, len(pak.entries)-1)
	for i := range blobs {
		start := pak.entries[i].offset
		end := pak.entries[i+1].offset
		blob := data[start:end]
		count := bytes.Count(blob, from)
		if count == 0 {
			blobs[i] = blob
			continue
		}
		blobs[i] = bytes.ReplaceAll(blob, from, to)
		replaced += count
	}
	if replaced == 0 {
		return data, 0, nil
	}

	out := pak.write(blobs)
	if _, err := parsePak(out); err != nil {
		return nil, 0, fmt.Errorf("rewritten pack: %w", err)
	}
	return out, replaced, nil
}

func suffixPair(encoding byte) (from, to []byte) {
	switch encoding {
	case encodingUTF8:
		return []byte(oldSuffix), []byte(newSuffix)
	case encodingUTF16:
		return utf16LE(oldSuffix), utf16LE(newSuffix)
	default:
		return nil, nil
	}
}

func utf16LE(s string) []byte {
	out := make([]byte, len(s)*2)
	for i := 0; i < len(s); i++ {
		out[i*2] = s[i]
	}
	return out
}

type entry struct {
	id     uint16
	offset uint32
}

type pakFile struct {
	version  uint32
	encoding byte
	aliases  []byte
	entries  []entry
}

func parsePak(data []byte) (pakFile, error) {
	if len(data) < 4 {
		return pakFile{}, fmt.Errorf("pack is too short")
	}
	version := binary.LittleEndian.Uint32(data[:4])
	var encoding byte
	var count, aliases int
	var header int
	switch version {
	case 4:
		if len(data) < 9 {
			return pakFile{}, fmt.Errorf("pack header is truncated")
		}
		count = int(binary.LittleEndian.Uint32(data[4:8]))
		encoding = data[8]
		header = 9
	case 5:
		if len(data) < 12 {
			return pakFile{}, fmt.Errorf("pack header is truncated")
		}
		encoding = data[4]
		count = int(binary.LittleEndian.Uint16(data[8:10]))
		aliases = int(binary.LittleEndian.Uint16(data[10:12]))
		header = 12
	default:
		return pakFile{}, fmt.Errorf("unsupported pack version %d", version)
	}
	if encoding != encodingUTF8 && encoding != encodingUTF16 && encoding != 0 {
		return pakFile{}, fmt.Errorf("unsupported pack encoding %d", encoding)
	}

	indexLen := (count + 1) * 6
	aliasLen := aliases * 4
	if header+indexLen+aliasLen > len(data) {
		return pakFile{}, fmt.Errorf("pack index is truncated")
	}

	entries := make([]entry, count+1)
	for i := range entries {
		off := header + i*6
		entries[i] = entry{
			id:     binary.LittleEndian.Uint16(data[off:]),
			offset: binary.LittleEndian.Uint32(data[off+2:]),
		}
		if i > 0 && entries[i].offset < entries[i-1].offset {
			return pakFile{}, fmt.Errorf("pack offsets go backwards")
		}
		if entries[i].offset > uint32(len(data)) {
			return pakFile{}, fmt.Errorf("pack offset past end of file")
		}
	}
	if entries[0].offset != uint32(header+indexLen+aliasLen) {
		return pakFile{}, fmt.Errorf("pack data does not follow the index")
	}
	if entries[len(entries)-1].offset != uint32(len(data)) {
		return pakFile{}, fmt.Errorf("pack size does not match the last offset")
	}

	aliasStart := header + indexLen
	return pakFile{
		version:  version,
		encoding: encoding,
		aliases:  append([]byte(nil), data[aliasStart:aliasStart+aliasLen]...),
		entries:  entries,
	}, nil
}

func (p pakFile) write(blobs [][]byte) []byte {
	count := len(p.entries) - 1
	header := 12
	if p.version == 4 {
		header = 9
	}
	indexLen := (count + 1) * 6
	size := header + indexLen + len(p.aliases)
	for _, blob := range blobs {
		size += len(blob)
	}
	out := make([]byte, 0, size)

	var head [12]byte
	binary.LittleEndian.PutUint32(head[:4], p.version)
	if p.version == 4 {
		binary.LittleEndian.PutUint32(head[4:8], uint32(count))
		head[8] = p.encoding
		out = append(out, head[:9]...)
	} else {
		head[4] = p.encoding
		binary.LittleEndian.PutUint16(head[8:10], uint16(count))
		binary.LittleEndian.PutUint16(head[10:12], uint16(len(p.aliases)/4))
		out = append(out, head[:]...)
	}

	offset := uint32(header + indexLen + len(p.aliases))
	var ent [6]byte
	for i, blob := range blobs {
		binary.LittleEndian.PutUint16(ent[:2], p.entries[i].id)
		binary.LittleEndian.PutUint32(ent[2:], offset)
		out = append(out, ent[:]...)
		offset += uint32(len(blob))
	}
	binary.LittleEndian.PutUint16(ent[:2], p.entries[count].id)
	binary.LittleEndian.PutUint32(ent[2:], offset)
	out = append(out, ent[:]...)
	out = append(out, p.aliases...)
	for _, blob := range blobs {
		out = append(out, blob...)
	}
	return out
}

func containsNewSuffix(data []byte) bool {
	plain := data
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		decoded, err := gunzip(data)
		if err != nil {
			return false
		}
		plain = decoded
	}
	pak, err := parsePak(plain)
	if err != nil {
		return false
	}
	_, to := suffixPair(pak.encoding)
	if to == nil {
		return false
	}
	for i := 0; i < len(pak.entries)-1; i++ {
		blob := plain[pak.entries[i].offset:pak.entries[i+1].offset]
		if bytes.Contains(blob, to) {
			return true
		}
	}
	return false
}

func gunzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
