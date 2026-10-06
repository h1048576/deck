package gui

import (
	"context"
	"embed"
	"encoding/base64"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

//go:embed all:scripts
var scriptsFS embed.FS

var ansiRegexp = regexp.MustCompile("\x1b\\[[0-9;]*m")

// ExtractScripts writes the embedded helper scripts to disk so PowerShell can
// invoke them by path (droid.ps1 -Background re-launches itself the same way).
func ExtractScripts(dir string) error {
	return fs.WalkDir(scriptsFS, "scripts", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(path, "scripts")))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		content, err := scriptsFS.ReadFile(path)
		if err != nil {
			return err
		}
		if existing, err := os.ReadFile(target); err == nil && string(existing) == string(content) {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, content, 0o644)
	})
}

// ScriptPath returns the extracted path of a helper script.
func ScriptPath(dir string, parts ...string) string {
	return filepath.Join(append([]string{dir}, parts...)...)
}

func psQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// psArgs mirrors the launcher's powershell invocation: the command travels as
// a base64-encoded UTF-16LE string so quoting never breaks.
func psArgs(command string) []string {
	prefix := "$ProgressPreference='SilentlyContinue'; [Console]::OutputEncoding=[Text.UTF8Encoding]::new(); $OutputEncoding=[Console]::OutputEncoding; "
	encoded := utf16.Encode([]rune(prefix + command))
	bytes := make([]byte, 0, len(encoded)*2)
	for _, unit := range encoded {
		bytes = append(bytes, byte(unit), byte(unit>>8))
	}
	return []string{"-NoProfile", "-NonInteractive", "-OutputFormat", "Text", "-ExecutionPolicy", "Bypass", "-EncodedCommand", base64.StdEncoding.EncodeToString(bytes)}
}

func powershellPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}

type psError struct {
	message string
	killed  bool
	code    string // process start failure code, e.g. ENOENT
	stderr  string
}

func (e *psError) Error() string { return e.message }

// psExec runs a PowerShell command and returns raw stdout (mirrors execFile).
func psExec(command string, timeout time.Duration) (string, string, *psError) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.Command(powershellPath(), psArgs(command)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), nil
	}
	failure := &psError{message: err.Error(), stderr: stderr.String()}
	if ctx.Err() == context.DeadlineExceeded {
		failure.killed = true
	}
	if strings.Contains(err.Error(), "executable file not found") {
		failure.code = "ENOENT"
	}
	return stdout.String(), stderr.String(), failure
}

// psOutput is the incremental PowerShell stream decoder: it strips ANSI
// escapes, decodes CLIXML <Objs> fragments and yields clean lines.
type psOutput struct {
	consume func(line string)
	buffer  strings.Builder
	reading bool
}

func newPSOutput(consume func(line string)) *psOutput {
	return &psOutput{consume: consume}
}

func decodeCLIXML(value string) string {
	replacer := strings.NewReplacer(
		"&lt;", "<", "&gt;", ">", "&quot;", "\"", "&apos;", "'", "&amp;", "&",
	)
	value = replacer.Replace(value)
	return hexPattern.ReplaceAllStringFunc(value, func(match string) string {
		code, err := strconv.ParseInt(match[2:6], 16, 32)
		if err != nil {
			return match
		}
		return string(rune(code))
	})
}

func (o *psOutput) emit(value string) {
	for _, line := range strings.Split(ansiRegexp.ReplaceAllString(value, ""), "\n") {
		if strings.TrimSpace(line) != "" {
			o.consume(line)
		}
	}
}

func (o *psOutput) write(value string) {
	o.buffer.WriteString(value)
	o.drain(false)
}

func (o *psOutput) drain(final bool) {
	buffer := o.buffer.String()
	for buffer != "" {
		buffer = strings.TrimPrefix(buffer, "#< CLIXML")
		start := 0
		if !o.reading {
			start = strings.Index(buffer, "<Objs")
		}
		if start >= 0 {
			if start > 0 {
				o.emit(buffer[:start])
				buffer = buffer[start:]
			}
			o.reading = true
			end := strings.Index(buffer, "</Objs>")
			match := sTagPattern.FindStringSubmatchIndex(buffer)
			// 信息流可能一直到进程退出才闭合；完整消息到达即交付，不等整个 XML 文档。
			if match != nil && (end < 0 || match[0] < end) {
				attrs, payload := buffer[match[2]:match[3]], buffer[match[4]:match[5]]
				if strings.Contains(attrs, `N="Message"`) || sAttrPattern.MatchString(attrs) {
					o.emit(decodeCLIXML(payload))
				}
				buffer = buffer[match[1]:]
				continue
			}
			if end < 0 {
				if final {
					buffer = ""
				}
				o.buffer.Reset()
				o.buffer.WriteString(buffer)
				return
			}
			buffer = buffer[end+7:]
			o.reading = false
			continue
		}
		newline := strings.Index(buffer, "\n")
		if newline < 0 {
			if final {
				o.emit(buffer)
				buffer = ""
			}
			o.buffer.Reset()
			o.buffer.WriteString(buffer)
			return
		}
		o.emit(buffer[:newline])
		buffer = buffer[newline+1:]
	}
	o.buffer.Reset()
}

var (
	sTagPattern  = regexp.MustCompile(`<S\b([^>]*)>([\s\S]*?)</S>`)
	sAttrPattern = regexp.MustCompile(`(?i)\bS="(?:Error|Warning|Information)"`)
	hexPattern   = regexp.MustCompile(`(?i)_x([0-9a-f]{4})_`)
)

// readLines streams raw chunks from a reader through the decoder.
func readLines(reader io.Reader, decoder *psOutput) {
	chunk := make([]byte, 32*1024)
	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			decoder.write(string(chunk[:n]))
		}
		if err != nil {
			return
		}
	}
}
