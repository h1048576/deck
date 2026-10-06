// Package jsonc implements position-aware JSONC parsing and editing.
// Edits splice the original text so comments and formatting outside the
// edited value are preserved byte-for-byte, mirroring the behavior the
// original app gets from jsonc-parser's modify/applyEdits.
package jsonc

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Obj is a JSON object that remembers key insertion order (JS semantics:
// assigning an existing key keeps its position).
type Obj struct {
	keys []string
	vals map[string]any
}

// NewObj creates an empty ordered object.
func NewObj() *Obj { return &Obj{vals: map[string]any{}} }

// Set assigns a value, keeping the original position of existing keys.
func (o *Obj) Set(key string, value any) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = value
}

// Delete removes a key.
func (o *Obj) Delete(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// Get returns a value by key.
func (o *Obj) Get(key string) (any, bool) {
	v, ok := o.vals[key]
	return v, ok
}

// Has reports whether the key exists.
func (o *Obj) Has(key string) bool { _, ok := o.vals[key]; return ok }

// Keys returns keys in insertion order.
func (o *Obj) Keys() []string { return append([]string(nil), o.keys...) }

// Clone deep-copies the object.
func (o *Obj) Clone() *Obj {
	out := NewObj()
	for _, k := range o.keys {
		out.Set(k, cloneValue(o.vals[k]))
	}
	return out
}

func cloneValue(v any) any {
	switch t := v.(type) {
	case *Obj:
		return t.Clone()
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = cloneValue(item)
		}
		return out
	default:
		return v
	}
}

// FromMap converts a Go map into an ordered object with sorted keys.
func FromMap(m map[string]any) *Obj {
	out := NewObj()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.Set(k, m[k])
	}
	return out
}

// Plain converts an Obj (recursively) into map[string]any.
func Plain(v any) any {
	switch t := v.(type) {
	case *Obj:
		out := make(map[string]any, len(t.keys))
		for _, k := range t.keys {
			out[k] = Plain(t.vals[k])
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = Plain(item)
		}
		return out
	default:
		return v
	}
}

// Equal compares two decoded values structurally.
func Equal(a, b any) bool {
	switch ta := a.(type) {
	case *Obj:
		tb, ok := b.(*Obj)
		if !ok || len(ta.keys) != len(tb.keys) {
			return false
		}
		for _, k := range ta.keys {
			bv, exists := tb.vals[k]
			if !exists || !Equal(ta.vals[k], bv) {
				return false
			}
		}
		return true
	case map[string]any:
		return Equal(FromMap(ta), b)
	case []any:
		tb, ok := b.([]any)
		if !ok || len(ta) != len(tb) {
			return false
		}
		for i := range ta {
			if !Equal(ta[i], tb[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

type nodeKind byte

const (
	kindObject nodeKind = iota
	kindArray
	kindScalar
)

type node struct {
	kind  nodeKind
	start int
	end   int
	props []prop  // kindObject
	items []*node // kindArray
	key   string  // kindScalar strings: decoded value
}

type prop struct {
	key   string
	start int // key token start
	val   *node
}

// Doc is a parsed JSONC document that supports text-preserving edits.
type Doc struct {
	text string
	root *node
}

type parser struct {
	text string
	pos  int
}

// Parse decodes a JSONC document into Go values.
func Parse(text string) (any, error) {
	doc, err := ParseDoc(text)
	if err != nil {
		return nil, err
	}
	return doc.Decode(), nil
}

// ParseDoc parses text into an editable document.
func ParseDoc(text string) (*Doc, error) {
	p := &parser{text: text}
	p.skipWhitespace()
	root, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	p.skipWhitespace()
	if p.pos != len(text) {
		return nil, fmt.Errorf("unexpected trailing content at offset %d", p.pos)
	}
	return &Doc{text: text, root: root}, nil
}

// Format carries source formatting for edits.
type Format struct {
	Unit string // indentation unit
	Eol  string // line ending
}

// DetectFormat infers indentation and line endings from the document.
func (d *Doc) DetectFormat() Format {
	unit := "  "
	text := d.text
	for i := 0; i < len(text); i++ {
		if text[i] != '\n' {
			continue
		}
		j := i + 1
		for j < len(text) && (text[j] == ' ' || text[j] == '\t') {
			j++
		}
		if j < len(text) && text[j] != '\n' && text[j] != '\r' {
			unit = text[i+1 : j]
			break
		}
	}
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	return Format{Unit: unit, Eol: eol}
}

// Decode returns the document's value tree.
func (d *Doc) Decode() any { return decodeNode(d, d.root) }

func decodeNode(d *Doc, n *node) any {
	switch n.kind {
	case kindObject:
		out := NewObj()
		for _, p := range n.props {
			out.Set(p.key, decodeNode(d, p.val))
		}
		return out
	case kindArray:
		out := make([]any, 0, len(n.items))
		for _, item := range n.items {
			out = append(out, decodeNode(d, item))
		}
		return out
	default:
		raw := strings.TrimSpace(d.text[n.start:n.end])
		switch {
		case strings.HasPrefix(raw, "\""):
			return unescapeString(raw)
		case raw == "true":
			return true
		case raw == "false":
			return false
		case raw == "null":
			return nil
		default:
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return raw
			}
			return value
		}
	}
}

// Get resolves a path (string keys, int array indexes) to a value.
func (d *Doc) Get(path ...any) (any, bool) {
	current := d.root
	for _, segment := range path {
		var next *node
		switch key := segment.(type) {
		case string:
			if current.kind != kindObject {
				return nil, false
			}
			for _, p := range current.props {
				if p.key == key {
					next = p.val
					break
				}
			}
		case int:
			if current.kind != kindArray || key < 0 || key >= len(current.items) {
				return nil, false
			}
			next = current.items[key]
		default:
			return nil, false
		}
		if next == nil {
			return nil, false
		}
		current = next
	}
	return decodeNode(d, current), true
}

// Set replaces the value at path, creating the final object key when
// missing. Array indexes may equal the length to append.
func (d *Doc) Set(path []any, value any, f Format) (string, error) {
	return d.edit(path, value, f, false)
}

// InsertArray appends a value to the array at path.
func (d *Doc) InsertArray(path []any, value any, f Format) (string, error) {
	return d.edit(path, value, f, true)
}

// Delete removes the value at path.
func (d *Doc) Delete(path []any, f Format) (string, error) {
	if len(path) == 0 {
		return "", fmt.Errorf("cannot delete the document root")
	}
	parent, ok := d.locate(path[:len(path)-1])
	if !ok || parent == nil {
		return "", fmt.Errorf("edit path not found")
	}
	switch last := path[len(path)-1].(type) {
	case string:
		if parent.kind != kindObject {
			return "", fmt.Errorf("expected object at path")
		}
		for _, p := range parent.props {
			if p.key == last {
				return d.removeRange(p.start, p.val.end), nil
			}
		}
		return d.text, nil
	case int:
		if parent.kind != kindArray {
			return "", fmt.Errorf("expected array at path")
		}
		if last < 0 || last >= len(parent.items) {
			return d.text, nil
		}
		return d.removeRange(parent.items[last].start, parent.items[last].end), nil
	default:
		return "", fmt.Errorf("invalid path segment")
	}
}

// removeRange splices out [start,end) extending to full lines and the
// adjacent comma so the surrounding formatting stays valid.
func (d *Doc) removeRange(start, end int) string {
	text := d.text
	// Extend start to the beginning of the line when the target owns its line.
	lineStart := start
	for lineStart > 0 && text[lineStart-1] != '\n' {
		lineStart--
	}
	if onlyWhitespace(text[lineStart:start]) {
		start = lineStart
	}
	// Prefer removing the following comma.
	commaEnd := d.commaAfter(end)
	if commaEnd >= 0 {
		cut := commaEnd
		for cut < len(text) && (text[cut] == ' ' || text[cut] == '\t') {
			cut++
		}
		return text[:start] + text[cut:]
	}
	// Otherwise remove a preceding comma when one exists.
	commaStart := d.prevCommaBefore(start)
	if commaStart >= 0 {
		return text[:commaStart] + text[end:]
	}
	return text[:start] + text[end:]
}

func onlyWhitespace(text string) bool {
	return strings.TrimLeft(text, " \t") == ""
}

func (d *Doc) commaAfter(from int) int {
	pos := from
	for pos < len(d.text) {
		switch d.text[pos] {
		case ' ', '\t', '\r', '\n':
			pos++
		case '/':
			next := d.skipComment(pos)
			if next < 0 {
				return -1
			}
			pos = next
		case ',':
			return pos + 1
		default:
			return -1
		}
	}
	return -1
}

func (d *Doc) prevCommaBefore(from int) int {
	pos := from - 1
	for pos >= 0 {
		switch d.text[pos] {
		case ' ', '\t', '\r', '\n':
			pos--
		case '/':
			// Block comment ending at pos? Simplest: treat as blocker.
			return -1
		case ',':
			return pos
		default:
			return -1
		}
	}
	return -1
}

func (d *Doc) skipComment(pos int) int {
	if pos+1 >= len(d.text) {
		return -1
	}
	switch d.text[pos+1] {
	case '/':
		for pos < len(d.text) && d.text[pos] != '\n' {
			pos++
		}
		return pos
	case '*':
		end := strings.Index(d.text[pos+2:], "*/")
		if end < 0 {
			return -1
		}
		return pos + 2 + end + 2
	default:
		return -1
	}
}

func (d *Doc) locate(path []any) (*node, bool) {
	current := d.root
	for _, segment := range path {
		var next *node
		switch key := segment.(type) {
		case string:
			if current.kind != kindObject {
				return nil, false
			}
			for _, p := range current.props {
				if p.key == key {
					next = p.val
					break
				}
			}
		case int:
			if current.kind != kindArray || key < 0 || key >= len(current.items) {
				return nil, false
			}
			next = current.items[key]
		default:
			return nil, false
		}
		if next == nil {
			return nil, false
		}
		current = next
	}
	return current, true
}

func (d *Doc) edit(path []any, value any, f Format, appendOnly bool) (string, error) {
	if len(path) == 0 {
		return "", fmt.Errorf("cannot replace the document root")
	}
	// Create missing intermediate objects one level at a time, mirroring
	// jsonc-parser's modify behavior.
	for depth := 1; depth < len(path); depth++ {
		if _, ok := d.locate(path[:depth]); ok {
			continue
		}
		parent, ok := d.locate(path[:depth-1])
		if !ok || parent == nil {
			return "", fmt.Errorf("edit path not found")
		}
		key, isString := path[depth-1].(string)
		if !isString || parent.kind != kindObject {
			return "", fmt.Errorf("edit path not found")
		}
		text := d.insertMember(parent, key, NewObj(), f)
		updated, err := ParseDoc(text)
		if err != nil {
			return "", err
		}
		*d = *updated
	}
	parent, ok := d.locate(path[:len(path)-1])
	if !ok || parent == nil {
		return "", fmt.Errorf("edit path not found")
	}
	switch last := path[len(path)-1].(type) {
	case string:
		if parent.kind != kindObject {
			return "", fmt.Errorf("expected object at path")
		}
		for _, p := range parent.props {
			if p.key == last {
				if appendOnly {
					return d.text, nil
				}
				replacement := formatValue(value, d.lineIndent(p.val.start), f)
				return d.text[:p.val.start] + replacement + d.text[p.val.end:], nil
			}
		}
		return d.insertMember(parent, last, value, f), nil
	case int:
		if parent.kind != kindArray {
			return "", fmt.Errorf("expected array at path")
		}
		if last >= 0 && last < len(parent.items) {
			if appendOnly {
				return d.text, nil
			}
			item := parent.items[last]
			replacement := formatValue(value, d.lineIndent(item.start), f)
			return d.text[:item.start] + replacement + d.text[item.end:], nil
		}
		if last == len(parent.items) || appendOnly {
			return d.insertElement(parent, value, f), nil
		}
		return "", fmt.Errorf("array index out of range")
	default:
		return "", fmt.Errorf("invalid path segment")
	}
}

func (d *Doc) lineIndent(pos int) string {
	lineStart := pos
	for lineStart > 0 && d.text[lineStart-1] != '\n' {
		lineStart--
	}
	if onlyWhitespace(d.text[lineStart:pos]) {
		return d.text[lineStart:pos]
	}
	// Value starts mid-line; indent nested lines from the line's own indent.
	if onlyWhitespace(d.text[:lineStart]) || lineStart == 0 {
		return ""
	}
	ws := lineStart
	for ws > 0 && (d.text[ws-1] == ' ' || d.text[ws-1] == '\t') {
		ws--
	}
	lineBegin := ws
	for lineBegin > 0 && d.text[lineBegin-1] != '\n' {
		lineBegin--
	}
	if onlyWhitespace(d.text[lineBegin:ws]) {
		return d.text[lineBegin:ws]
	}
	return ""
}

// insertMember adds "key": value to an object before its closing brace.
func (d *Doc) insertMember(parent *node, key string, value any, f Format) string {
	closing := parent.end - 1 // position of '}'
	childIndent := d.lineIndent(closing) + f.Unit
	valueText := formatValue(value, childIndent, f)
	propText := quote(key) + ": " + valueText
	insertAt, needsComma := d.lastSignificantBefore(closing)
	if insertAt == parent.start && strings.TrimSpace(d.text[parent.start+1:closing]) == "" && !strings.Contains(d.text[parent.start+1:closing], "\n") {
		// Inline empty object: place the property on its own lines.
		parentIndent := d.lineIndent(parent.start)
		return d.text[:closing] + f.Eol + childIndent + propText + f.Eol + parentIndent + d.text[closing:]
	}
	text := ""
	if needsComma {
		text += ","
	}
	text += f.Eol + childIndent + propText
	return d.text[:insertAt+1] + text + d.text[insertAt+1:]
}

// insertElement appends a value to an array before its closing bracket.
func (d *Doc) insertElement(parent *node, value any, f Format) string {
	closing := parent.end - 1
	childIndent := d.lineIndent(closing) + f.Unit
	valueText := formatValue(value, childIndent, f)
	insertAt, needsComma := d.lastSignificantBefore(closing)
	if insertAt == parent.start && strings.TrimSpace(d.text[parent.start+1:closing]) == "" && !strings.Contains(d.text[parent.start+1:closing], "\n") {
		parentIndent := d.lineIndent(parent.start)
		return d.text[:closing] + f.Eol + childIndent + valueText + f.Eol + parentIndent + d.text[closing:]
	}
	text := ""
	if needsComma {
		text += ","
	}
	text += f.Eol + childIndent + valueText
	return d.text[:insertAt+1] + text + d.text[insertAt+1:]
}

// lastSignificantBefore finds the position of the last non-whitespace,
// non-comment character strictly before end.
func (d *Doc) lastSignificantBefore(end int) (int, bool) {
	last := -1
	for pos := 0; pos < end; {
		ch := d.text[pos]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			pos++
			continue
		}
		if ch == '/' {
			if next := d.skipComment(pos); next >= 0 && next <= end {
				pos = next
				continue
			}
		}
		last = pos
		pos++
		if ch == '"' {
			for pos < end {
				last = pos
				if d.text[pos] == '\\' {
					pos += 2
					continue
				}
				pos++
				if d.text[last] == '"' {
					break
				}
			}
		}
	}
	if last < 0 {
		return -1, false
	}
	ch := d.text[last]
	return last, ch != ',' && ch != '{' && ch != '['
}

// Canonical serializes a decoded value compactly with objects in their
// insertion order; used for stable revision hashes.
func Canonical(value any) string {
	var sb strings.Builder
	writeCanonical(&sb, value)
	return sb.String()
}

// Pretty renders a decoded value like JSON.stringify(value, null, 2),
// preserving object insertion order.
func Pretty(value any) string {
	return formatValue(value, "", Format{Unit: "  ", Eol: "\n"})
}

func writeCanonical(sb *strings.Builder, value any) {
	switch v := value.(type) {
	case *Obj:
		sb.WriteByte('{')
		for i, key := range v.keys {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(quote(key))
			sb.WriteByte(':')
			writeCanonical(sb, v.vals[key])
		}
		sb.WriteByte('}')
	case []any:
		sb.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				sb.WriteByte(',')
			}
			writeCanonical(sb, item)
		}
		sb.WriteByte(']')
	default:
		sb.WriteString(formatValue(value, "", Format{Unit: "", Eol: "\n"}))
	}
}

func formatValue(value any, indent string, f Format) string {
	var sb strings.Builder
	writeValue(&sb, value, indent, f)
	return sb.String()
}

func writeValue(sb *strings.Builder, value any, indent string, f Format) {
	switch v := value.(type) {
	case nil:
		sb.WriteString("null")
	case bool:
		if v {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case float64:
		sb.WriteString(formatNumber(v))
	case int:
		sb.WriteString(strconv.Itoa(v))
	case string:
		sb.WriteString(quote(v))
	case *Obj:
		if len(v.keys) == 0 {
			sb.WriteString("{}")
			return
		}
		sb.WriteString("{")
		for i, key := range v.keys {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(f.Eol)
			sb.WriteString(indent + f.Unit)
			sb.WriteString(quote(key))
			sb.WriteString(": ")
			writeValue(sb, v.vals[key], indent+f.Unit, f)
		}
		sb.WriteString(f.Eol)
		sb.WriteString(indent)
		sb.WriteString("}")
	case map[string]any:
		writeValue(sb, FromMap(v), indent, f)
	case []any:
		if len(v) == 0 {
			sb.WriteString("[]")
			return
		}
		sb.WriteString("[")
		for i, item := range v {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(f.Eol)
			sb.WriteString(indent + f.Unit)
			writeValue(sb, item, indent+f.Unit, f)
		}
		sb.WriteString(f.Eol)
		sb.WriteString(indent)
		sb.WriteString("]")
	default:
		text, err := marshalPlain(value)
		if err != nil {
			sb.WriteString("null")
			return
		}
		sb.Write(text)
	}
}

func marshalPlain(value any) ([]byte, error) {
	return json.Marshal(value)
}

func formatNumber(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func quote(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString("\\\"")
		case '\\':
			sb.WriteString("\\\\")
		case '\n':
			sb.WriteString("\\n")
		case '\r':
			sb.WriteString("\\r")
		case '\t':
			sb.WriteString("\\t")
		case '\b':
			sb.WriteString("\\b")
		case '\f':
			sb.WriteString("\\f")
		default:
			if r < 0x20 {
				sb.WriteString(fmt.Sprintf("\\u%04x", r))
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

func unescapeString(raw string) string {
	body := raw[1 : len(raw)-1]
	var sb strings.Builder
	for i := 0; i < len(body); i++ {
		ch := body[i]
		if ch != '\\' || i+1 >= len(body) {
			sb.WriteByte(ch)
			continue
		}
		i++
		switch body[i] {
		case '"':
			sb.WriteByte('"')
		case '\\':
			sb.WriteByte('\\')
		case '/':
			sb.WriteByte('/')
		case 'b':
			sb.WriteByte('\b')
		case 'f':
			sb.WriteByte('\f')
		case 'n':
			sb.WriteByte('\n')
		case 'r':
			sb.WriteByte('\r')
		case 't':
			sb.WriteByte('\t')
		case 'u':
			if i+4 < len(body) {
				code, err := strconv.ParseUint(body[i+1:i+5], 16, 32)
				if err == nil {
					i += 4
					r := rune(code)
					if utf16.IsSurrogate(r) && i+6 < len(body) && body[i+1] == '\\' && body[i+2] == 'u' {
						if low, err := strconv.ParseUint(body[i+3:i+7], 16, 32); err == nil {
							combined := utf16.DecodeRune(r, rune(low))
							if combined != 0xFFFD {
								i += 6
								sb.WriteRune(combined)
								continue
							}
						}
					}
					sb.WriteRune(r)
				}
			}
		default:
			sb.WriteByte('\\')
			sb.WriteByte(body[i])
		}
	}
	return sb.String()
}

// --- scanner ---

func (p *parser) skipWhitespace() {
	for p.pos < len(p.text) {
		ch := p.text[p.pos]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n':
			p.pos++
		case ch == '/' && p.pos+1 < len(p.text) && p.text[p.pos+1] == '/':
			for p.pos < len(p.text) && p.text[p.pos] != '\n' {
				p.pos++
			}
		case ch == '/' && p.pos+1 < len(p.text) && p.text[p.pos+1] == '*':
			end := strings.Index(p.text[p.pos+2:], "*/")
			if end < 0 {
				return
			}
			p.pos += 2 + end + 2
		default:
			return
		}
	}
}

func (p *parser) parseValue() (*node, error) {
	p.skipWhitespace()
	if p.pos >= len(p.text) {
		return nil, fmt.Errorf("unexpected end of input")
	}
	switch p.text[p.pos] {
	case '{':
		return p.parseObject()
	case '[':
		return p.parseArray()
	case '"':
		start := p.pos
		if _, err := p.parseString(); err != nil {
			return nil, err
		}
		return &node{kind: kindScalar, start: start, end: p.pos}, nil
	default:
		start := p.pos
		for p.pos < len(p.text) {
			ch := p.text[p.pos]
			if (ch >= '0' && ch <= '9') || ch == '-' || ch == '+' || ch == '.' || ch == 'e' || ch == 'E' ||
				(ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
				p.pos++
				continue
			}
			break
		}
		raw := p.text[start:p.pos]
		if raw == "" {
			return nil, fmt.Errorf("invalid value at offset %d", start)
		}
		switch raw {
		case "true", "false", "null":
		default:
			if _, err := strconv.ParseFloat(raw, 64); err != nil {
				return nil, fmt.Errorf("invalid literal %q at offset %d", raw, start)
			}
		}
		return &node{kind: kindScalar, start: start, end: p.pos}, nil
	}
}

func (p *parser) parseObject() (*node, error) {
	start := p.pos
	p.pos++ // consume '{'
	out := &node{kind: kindObject, start: start}
	for {
		p.skipWhitespace()
		if p.pos >= len(p.text) {
			return nil, fmt.Errorf("unterminated object at offset %d", start)
		}
		if p.text[p.pos] == '}' {
			p.pos++
			out.end = p.pos
			return out, nil
		}
		if p.text[p.pos] != '"' {
			return nil, fmt.Errorf("expected object key at offset %d", p.pos)
		}
		keyStart := p.pos
		key, err := p.parseString()
		if err != nil {
			return nil, err
		}
		p.skipWhitespace()
		if p.pos >= len(p.text) || p.text[p.pos] != ':' {
			return nil, fmt.Errorf("expected ':' after key at offset %d", p.pos)
		}
		p.pos++
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out.props = append(out.props, prop{key: key, start: keyStart, val: value})
		p.skipWhitespace()
		if p.pos < len(p.text) && p.text[p.pos] == ',' {
			p.pos++
			// trailing comma allowed
			p.skipWhitespace()
			if p.pos < len(p.text) && p.text[p.pos] == '}' {
				p.pos++
				out.end = p.pos
				return out, nil
			}
			continue
		}
		if p.pos >= len(p.text) || p.text[p.pos] != '}' {
			return nil, fmt.Errorf("expected ',' or '}' at offset %d", p.pos)
		}
	}
}

func (p *parser) parseArray() (*node, error) {
	start := p.pos
	p.pos++ // consume '['
	out := &node{kind: kindArray, start: start}
	for {
		p.skipWhitespace()
		if p.pos >= len(p.text) {
			return nil, fmt.Errorf("unterminated array at offset %d", start)
		}
		if p.text[p.pos] == ']' {
			p.pos++
			out.end = p.pos
			return out, nil
		}
		item, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out.items = append(out.items, item)
		p.skipWhitespace()
		if p.pos < len(p.text) && p.text[p.pos] == ',' {
			p.pos++
			p.skipWhitespace()
			if p.pos < len(p.text) && p.text[p.pos] == ']' {
				p.pos++
				out.end = p.pos
				return out, nil
			}
			continue
		}
		if p.pos >= len(p.text) || p.text[p.pos] != ']' {
			return nil, fmt.Errorf("expected ',' or ']' at offset %d", p.pos)
		}
	}
}

func (p *parser) parseString() (string, error) {
	start := p.pos
	p.pos++ // consume '"'
	var sb strings.Builder
	for p.pos < len(p.text) {
		ch := p.text[p.pos]
		switch ch {
		case '"':
			p.pos++
			return sb.String(), nil
		case '\\':
			if p.pos+1 >= len(p.text) {
				return "", fmt.Errorf("unterminated escape at offset %d", p.pos)
			}
			esc := p.text[p.pos+1]
			p.pos += 2
			switch esc {
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			case '/':
				sb.WriteByte('/')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'u':
				if p.pos+4 > len(p.text) {
					return "", fmt.Errorf("invalid unicode escape at offset %d", p.pos)
				}
				code, err := strconv.ParseUint(p.text[p.pos:p.pos+4], 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid unicode escape at offset %d", p.pos)
				}
				p.pos += 4
				r := rune(code)
				if utf16.IsSurrogate(r) && p.pos+6 <= len(p.text) && p.text[p.pos] == '\\' && p.text[p.pos+1] == 'u' {
					if low, err := strconv.ParseUint(p.text[p.pos+2:p.pos+6], 16, 32); err == nil {
						combined := utf16.DecodeRune(r, rune(low))
						if combined != 0xFFFD {
							p.pos += 6
							sb.WriteRune(combined)
							continue
						}
					}
				}
				sb.WriteRune(r)
			default:
				return "", fmt.Errorf("invalid escape \\%c at offset %d", esc, p.pos)
			}
		default:
			sb.WriteByte(ch)
			p.pos++
		}
	}
	return "", fmt.Errorf("unterminated string at offset %d", start)
}

// DetectFormatOf infers indentation and line endings from raw JSONC text.
func DetectFormatOf(text string) Format {
	unit := "  "
	for i := 0; i < len(text); i++ {
		if text[i] != '\n' {
			continue
		}
		j := i + 1
		for j < len(text) && (text[j] == ' ' || text[j] == '\t') {
			j++
		}
		if j < len(text) && text[j] != '\n' && text[j] != '\r' {
			unit = text[i+1 : j]
			break
		}
	}
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	return Format{Unit: unit, Eol: eol}
}
