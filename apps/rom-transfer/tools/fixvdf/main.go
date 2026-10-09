package main

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"strings"
)

const (
	vdfTypeDict   = 0x00
	vdfTypeString = 0x01
	vdfTypeInt32  = 0x02
	vdfTypeEnd    = 0x08
)

type vdfItem struct {
	typ      byte
	key      string
	strVal   string
	intVal   int32
	children []*vdfItem
}

func readCString(data []byte, pos int) (string, int) {
	end := pos
	for end < len(data) && data[end] != 0 {
		end++
	}
	return string(data[pos:end]), end + 1
}

func readVDF(data []byte, pos int) ([]*vdfItem, int, error) {
	var items []*vdfItem
	for pos < len(data) {
		typ := data[pos]
		pos++
		if typ == vdfTypeEnd {
			break
		}
		if pos >= len(data) {
			break
		}
		key, next := readCString(data, pos)
		pos = next
		item := &vdfItem{typ: typ, key: key}
		switch typ {
		case vdfTypeDict:
			children, after, err := readVDF(data, pos)
			if err != nil {
				return nil, after, err
			}
			item.children = children
			pos = after
		case vdfTypeString:
			val, after := readCString(data, pos)
			item.strVal = val
			pos = after
		case vdfTypeInt32:
			if pos+4 > len(data) {
				return nil, pos, fmt.Errorf("unexpected EOF reading int32")
			}
			item.intVal = int32(binary.LittleEndian.Uint32(data[pos : pos+4]))
			pos += 4
		default:
			return nil, pos, fmt.Errorf("unknown type 0x%02x", typ)
		}
		items = append(items, item)
	}
	return items, pos, nil
}

// writeVDF writes a list of items terminated by \x08.
// For dict items, the children are written by a recursive call which already
// ends with \x08 — that single \x08 is the dict's end marker.
func writeVDF(items []*vdfItem) []byte {
	var buf []byte
	for _, item := range items {
		buf = append(buf, item.typ)
		buf = append(buf, []byte(item.key)...)
		buf = append(buf, 0)
		switch item.typ {
		case vdfTypeDict:
			// writeVDF(children) already ends with \x08 which closes this dict.
			// Do NOT add another vdfTypeEnd here.
			buf = append(buf, writeVDF(item.children)...)
		case vdfTypeString:
			buf = append(buf, []byte(item.strVal)...)
			buf = append(buf, 0)
		case vdfTypeInt32:
			var tmp [4]byte
			binary.LittleEndian.PutUint32(tmp[:], uint32(item.intVal))
			buf = append(buf, tmp[:]...)
		}
	}
	buf = append(buf, vdfTypeEnd) // closes this list (acts as end marker for the parent dict)
	return buf
}

func writeShortcuts(entries []*vdfItem) []byte {
	for i, e := range entries {
		e.key = fmt.Sprintf("%d", i)
	}
	inner := writeVDF(entries)
	var buf []byte
	buf = append(buf, vdfTypeDict)
	buf = append(buf, []byte("shortcuts")...)
	buf = append(buf, 0)
	buf = append(buf, inner...)
	buf = append(buf, vdfTypeEnd) // root closing \x08
	return buf
}

func computeAppID(exe, name string) int32 {
	quotedExe := "\"" + exe + "\""
	crc := crc32.ChecksumIEEE([]byte(quotedExe + name))
	return int32(crc | 0x80000000)
}

func vdfStr(key, val string) *vdfItem  { return &vdfItem{typ: vdfTypeString, key: key, strVal: val} }
func vdfInt(key string, val int32) *vdfItem { return &vdfItem{typ: vdfTypeInt32, key: key, intVal: val} }
func vdfDict(key string, children []*vdfItem) *vdfItem {
	return &vdfItem{typ: vdfTypeDict, key: key, children: children}
}

func makeShortcut(name, exe, startDir string) *vdfItem {
	appID := computeAppID(exe, name)
	return vdfDict("", []*vdfItem{
		vdfInt("appid", appID),
		vdfStr("AppName", name),
		vdfStr("Exe", "\""+exe+"\""),
		vdfStr("StartDir", startDir), // no quotes on StartDir
		vdfStr("icon", ""),
		vdfStr("ShortcutPath", ""),
		vdfStr("LaunchOptions", ""),
		vdfInt("IsHidden", 0),
		vdfInt("AllowDesktopConfig", 1),
		vdfInt("AllowOverlay", 1),
		vdfInt("OpenVR", 0),
		vdfInt("Devkit", 0),
		vdfStr("DevkitGameID", ""),
		vdfInt("LastPlayTime", 0),
		vdfDict("tags", nil),
	})
}

func entryName(e *vdfItem) (name, exe, startDir string) {
	for _, c := range e.children {
		switch strings.ToLower(c.key) {
		case "appname":
			name = c.strVal
		case "exe":
			exe = c.strVal
		case "startdir":
			startDir = c.strVal
		}
	}
	return
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: fixvdf <input.vdf> <output.vdf>")
		os.Exit(1)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}

	items, endPos, err := readVDF(data, 0)
	if err != nil || len(items) == 0 || items[0].key != "shortcuts" {
		fmt.Fprintln(os.Stderr, "not a valid shortcuts.vdf")
		os.Exit(1)
	}

	entries := items[0].children
	fmt.Printf("Parsed %d entries from main structure (stopped at offset 0x%x):\n", len(entries), endPos)
	for _, e := range entries {
		n, ex, sd := entryName(e)
		fmt.Printf("  %s: AppName=%q StartDir=%q\n", e.key, n, sd)
		_ = ex
	}

	// Scan for orphaned entries after endPos (appended outside the structure by our buggy writer)
	fmt.Printf("\nScanning for orphaned entries after offset 0x%x...\n", endPos)
	pos := endPos
	// skip any trailing \x08 bytes
	for pos < len(data) && data[pos] == vdfTypeEnd {
		pos++
	}
	orphans, _, _ := readVDF(data, pos)
	for _, o := range orphans {
		if o.typ != vdfTypeDict {
			continue
		}
		n, ex, sd := entryName(o)
		fmt.Printf("  Found orphaned entry %q: AppName=%q Exe=%q StartDir=%q\n", o.key, n, ex, sd)
		// Fix: rebuild with correct StartDir (strip quotes) and add appid
		exeClean := strings.Trim(ex, "\"")
		sdClean  := strings.Trim(sd, "\"")
		fixed := makeShortcut(n, exeClean, sdClean)
		entries = append(entries, fixed)
		fmt.Printf("  → rebuilt with appid=%d, StartDir=%q\n", fixed.children[0].intVal, sdClean)
	}

	// Also fix any already-parsed entries with quoted StartDir
	for _, e := range entries {
		for _, c := range e.children {
			if strings.ToLower(c.key) == "startdir" {
				if len(c.strVal) >= 2 && c.strVal[0] == '"' && c.strVal[len(c.strVal)-1] == '"' {
					c.strVal = c.strVal[1 : len(c.strVal)-1]
				}
			}
		}
	}

	out := writeShortcuts(entries)
	if err := os.WriteFile(os.Args[2], out, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}

	// Verify by re-reading
	check, _, _ := readVDF(out, 0)
	fmt.Printf("\nVerification — entries in output:\n")
	if len(check) > 0 && check[0].key == "shortcuts" {
		for _, e := range check[0].children {
			n, ex, sd := entryName(e)
			fmt.Printf("  %s: AppName=%q Exe=%q StartDir=%q\n", e.key, n, ex, sd)
		}
	}
	fmt.Println("\nWrote", os.Args[2])
}
