package brand

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestRewriteTitleSuffix(t *testing.T) {
	original := buildPak(t, map[uint16]string{
		10: "$1 - Chromium",
		11: "$1 - Chromium Beta",
		12: "sign in to Chromium",
		13: "Task Manager - Chromium",
	}, map[uint16]uint16{
		20: 0,
	})

	out, n, err := rewrite(original)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("replacements %d", n)
	}

	got := readResources(t, out)
	if got[10] != "$1 - SafeRuBro" {
		t.Fatalf("title %q", got[10])
	}
	if got[11] != "$1 - SafeRuBro Beta" {
		t.Fatalf("beta title %q", got[11])
	}
	if got[12] != "sign in to Chromium" {
		t.Fatalf("unrelated string changed to %q", got[12])
	}
	if got[13] != "Task Manager - SafeRuBro" {
		t.Fatalf("task manager %q", got[13])
	}
	if got[20] != got[10] {
		t.Fatalf("alias %q, canonical %q", got[20], got[10])
	}

	again, n, err := rewrite(out)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || !bytes.Equal(again, out) {
		t.Fatalf("second rewrite changed the pack (%d)", n)
	}
}

func TestRewriteGzip(t *testing.T) {
	plain := buildPak(t, map[uint16]string{1: "$1 - Chromium"}, nil)
	packed, err := gzipBytes(plain)
	if err != nil {
		t.Fatal(err)
	}
	out, n, err := rewrite(packed)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("replacements %d", n)
	}
	decoded, err := gunzip(out)
	if err != nil {
		t.Fatal(err)
	}
	if readResources(t, decoded)[1] != "$1 - SafeRuBro" {
		t.Fatalf("%q", readResources(t, decoded)[1])
	}
}

func TestRewriteLeavesUnrelatedPack(t *testing.T) {
	original := buildPak(t, map[uint16]string{1: "New Tab"}, nil)
	out, n, err := rewrite(original)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || !bytes.Equal(out, original) {
		t.Fatalf("unrelated pack changed (%d)", n)
	}
}

func buildPak(t *testing.T, resources map[uint16]string, aliases map[uint16]uint16) []byte {
	t.Helper()
	ids := make([]uint16, 0, len(resources))
	for id := range resources {
		ids = append(ids, id)
	}
	for i := 1; i < len(ids); i++ {
		j := i
		for j > 0 && ids[j] < ids[j-1] {
			ids[j], ids[j-1] = ids[j-1], ids[j]
			j--
		}
	}

	aliasIDs := make([]uint16, 0, len(aliases))
	for id := range aliases {
		aliasIDs = append(aliasIDs, id)
	}
	for i := 1; i < len(aliasIDs); i++ {
		j := i
		for j > 0 && aliasIDs[j] < aliasIDs[j-1] {
			aliasIDs[j], aliasIDs[j-1] = aliasIDs[j-1], aliasIDs[j]
			j--
		}
	}

	header := 12
	indexLen := (len(ids) + 1) * 6
	aliasLen := len(aliasIDs) * 4
	offset := header + indexLen + aliasLen
	var buf bytes.Buffer
	var head [12]byte
	binary.LittleEndian.PutUint32(head[:4], 5)
	head[4] = encodingUTF8
	binary.LittleEndian.PutUint16(head[8:10], uint16(len(ids)))
	binary.LittleEndian.PutUint16(head[10:12], uint16(len(aliasIDs)))
	buf.Write(head[:])

	var ent [6]byte
	cursor := offset
	for _, id := range ids {
		binary.LittleEndian.PutUint16(ent[:2], id)
		binary.LittleEndian.PutUint32(ent[2:], uint32(cursor))
		buf.Write(ent[:])
		cursor += len(resources[id])
	}
	binary.LittleEndian.PutUint16(ent[:2], 0)
	binary.LittleEndian.PutUint32(ent[2:], uint32(cursor))
	buf.Write(ent[:])

	var alias [4]byte
	for _, id := range aliasIDs {
		binary.LittleEndian.PutUint16(alias[:2], id)
		binary.LittleEndian.PutUint16(alias[2:], aliases[id])
		buf.Write(alias[:])
	}
	for _, id := range ids {
		buf.WriteString(resources[id])
	}
	return buf.Bytes()
}

func readResources(t *testing.T, data []byte) map[uint16]string {
	t.Helper()
	pak, err := parsePak(data)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[uint16]string, len(pak.entries)-1)
	for i := 0; i < len(pak.entries)-1; i++ {
		blob := data[pak.entries[i].offset:pak.entries[i+1].offset]
		out[pak.entries[i].id] = string(blob)
	}
	for i := 0; i+4 <= len(pak.aliases); i += 4 {
		id := binary.LittleEndian.Uint16(pak.aliases[i:])
		index := binary.LittleEndian.Uint16(pak.aliases[i+2:])
		if int(index) >= len(pak.entries)-1 {
			t.Fatalf("alias %d index %d", id, index)
		}
		out[id] = out[pak.entries[index].id]
	}
	return out
}
