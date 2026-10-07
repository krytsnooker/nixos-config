//go:build windows

package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
)

// ── Steam account discovery ───────────────────────────────────────────────────

type SteamAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// steam64ToAccountID converts a Steam64 ID to the userdata folder ID.
func steam64ToAccountID(steam64 uint64) uint64 {
	return steam64 - 76561197960265728
}

// FindSteamAccounts returns all Steam accounts found under steamPath/userdata/.
// Display names are read from steamPath/config/loginusers.vdf.
func FindSteamAccounts(steamPath string) ([]SteamAccount, error) {
	userdataDir := filepath.Join(steamPath, "userdata")
	entries, err := os.ReadDir(userdataDir)
	if err != nil {
		return nil, fmt.Errorf("cannot read userdata: %w", err)
	}

	// Build accountID → display name map from loginusers.vdf (text VDF).
	names := parseLoginUsers(filepath.Join(steamPath, "config", "loginusers.vdf"))

	var accounts []SteamAccount
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		name, ok := names[id]
		if !ok {
			name = id
		}
		accounts = append(accounts, SteamAccount{ID: id, Name: name})
	}
	return accounts, nil
}

// parseLoginUsers reads a text VDF file and returns a map of
// accountID (string) → PersonaName.
// The file has blocks like:
//
//	"76561198012345678"
//	{
//	    "PersonaName"    "Alice"
//	    ...
//	}
func parseLoginUsers(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)

	var currentSteam64 string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "{" || line == "\"users\"" || line == "}" {
			continue
		}
		key, val, ok := splitVDFLine(line)
		if !ok {
			// bare quoted token — likely a Steam64 key
			tok := strings.Trim(line, "\"")
			if len(tok) == 17 && strings.HasPrefix(tok, "7656") {
				currentSteam64 = tok
			}
			continue
		}
		if strings.EqualFold(key, "PersonaName") && currentSteam64 != "" {
			// Convert steam64 to account ID.
			var s64 uint64
			fmt.Sscanf(currentSteam64, "%d", &s64)
			accountID := fmt.Sprintf("%d", steam64ToAccountID(s64))
			result[accountID] = val
			currentSteam64 = ""
		}
	}
	return result
}

// splitVDFLine splits a line like `"Key"   "Value"` into key, value.
func splitVDFLine(line string) (string, string, bool) {
	if len(line) == 0 || line[0] != '"' {
		return "", "", false
	}
	// find closing quote of key
	end := strings.Index(line[1:], "\"")
	if end < 0 {
		return "", "", false
	}
	key := line[1 : end+1]
	rest := strings.TrimSpace(line[end+2:])
	if len(rest) < 2 || rest[0] != '"' || rest[len(rest)-1] != '"' {
		return "", "", false
	}
	val := rest[1 : len(rest)-1]
	return key, val, true
}

// ── Binary VDF (shortcuts.vdf) ───────────────────────────────────────────────
//
// Format:
//   \x00 key \x00 { ... \x08 }   — dict/object
//   \x01 key \x00 value \x00     — string
//   \x02 key \x00 int32le        — int32
//   \x08                         — end of dict
//
// The root of shortcuts.vdf is an unnamed dict. Inside it, each entry is
// a dict keyed "0", "1", "2", … with fields: AppName, Exe, StartDir, etc.

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

// readCString reads a null-terminated string from data[pos:].
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
		if pos >= len(data) {
			break
		}
		typ := data[pos]
		pos++
		if typ == vdfTypeEnd {
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
			return nil, pos, fmt.Errorf("unknown VDF type 0x%02x at offset %d", typ, pos-1)
		}
		items = append(items, item)
	}
	return items, pos, nil
}

func writeVDF(items []*vdfItem) []byte {
	var buf []byte
	for _, item := range items {
		buf = append(buf, item.typ)
		buf = append(buf, []byte(item.key)...)
		buf = append(buf, 0)
		switch item.typ {
		case vdfTypeDict:
			buf = append(buf, writeVDF(item.children)...)
			buf = append(buf, vdfTypeEnd)
		case vdfTypeString:
			buf = append(buf, []byte(item.strVal)...)
			buf = append(buf, 0)
		case vdfTypeInt32:
			var tmp [4]byte
			binary.LittleEndian.PutUint32(tmp[:], uint32(item.intVal))
			buf = append(buf, tmp[:]...)
		}
	}
	buf = append(buf, vdfTypeEnd)
	return buf
}

func shortcutsPath(steamPath, userID string) string {
	return filepath.Join(steamPath, "userdata", userID, "config", "shortcuts.vdf")
}

func readShortcuts(path string) ([]*vdfItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) < 2 {
		return nil, nil
	}
	// Root is \x00 "shortcuts" \x00 { ... \x08 } \x08
	// Skip the outer wrapper and work with just the inner items.
	// We re-wrap on write.
	items, _, err := readVDF(data, 0)
	if err != nil {
		return nil, fmt.Errorf("parse shortcuts.vdf: %w", err)
	}
	// items[0] should be the "shortcuts" dict
	if len(items) == 0 || items[0].key != "shortcuts" {
		return nil, nil
	}
	return items[0].children, nil
}

func writeShortcuts(path string, entries []*vdfItem) error {
	// Renumber entries 0, 1, 2, …
	for i, e := range entries {
		e.key = fmt.Sprintf("%d", i)
	}
	inner := writeVDF(entries)
	// Wrap: \x00 "shortcuts" \x00 <inner> \x08
	var buf []byte
	buf = append(buf, vdfTypeDict)
	buf = append(buf, []byte("shortcuts")...)
	buf = append(buf, 0)
	buf = append(buf, inner...)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0644)
}

func vdfString(key, val string) *vdfItem {
	return &vdfItem{typ: vdfTypeString, key: key, strVal: val}
}

func vdfInt(key string, val int32) *vdfItem {
	return &vdfItem{typ: vdfTypeInt32, key: key, intVal: val}
}

// AddSteamShortcut adds (or overwrites) a non-Steam game shortcut.
func AddSteamShortcut(steamPath, userID, name, exe, startDir string) error {
	path := shortcutsPath(steamPath, userID)
	entries, err := readShortcuts(path)
	if err != nil {
		return err
	}

	quotedExe := "\"" + exe + "\""
	appID := int32(crc32.ChecksumIEEE([]byte(quotedExe+name)) | 0x80000000)

	newEntry := &vdfItem{
		typ: vdfTypeDict,
		children: []*vdfItem{
			vdfInt("appid", appID),
			vdfString("AppName", name),
			vdfString("Exe", quotedExe),
			vdfString("StartDir", "\""+startDir+"\""),
			vdfString("icon", ""),
			vdfString("ShortcutPath", ""),
			vdfString("LaunchOptions", ""),
			vdfInt("IsHidden", 0),
			vdfInt("AllowDesktopConfig", 1),
			vdfInt("AllowOverlay", 1),
			vdfInt("OpenVR", 0),
			vdfInt("Devkit", 0),
			vdfString("DevkitGameID", ""),
			vdfInt("LastPlayTime", 0),
			vdfItem2("tags"),
		},
	}

	// Check for existing entry with same AppName.
	for i, e := range entries {
		for _, c := range e.children {
			if c.typ == vdfTypeString && strings.EqualFold(c.key, "AppName") && c.strVal == name {
				entries[i] = newEntry
				return writeShortcuts(path, entries)
			}
		}
	}

	entries = append(entries, newEntry)
	return writeShortcuts(path, entries)
}

// vdfItem2 returns an empty dict item (used for the "tags" field).
func vdfItem2(key string) *vdfItem {
	return &vdfItem{typ: vdfTypeDict, key: key, children: nil}
}

// RemoveSteamShortcut removes all shortcuts with the given AppName.
func RemoveSteamShortcut(steamPath, userID, name string) error {
	path := shortcutsPath(steamPath, userID)
	entries, err := readShortcuts(path)
	if err != nil {
		return err
	}

	filtered := entries[:0]
	for _, e := range entries {
		keep := true
		for _, c := range e.children {
			if c.typ == vdfTypeString && strings.EqualFold(c.key, "AppName") && c.strVal == name {
				keep = false
				break
			}
		}
		if keep {
			filtered = append(filtered, e)
		}
	}

	if len(filtered) == len(entries) {
		return nil // nothing to remove
	}
	return writeShortcuts(path, filtered)
}
