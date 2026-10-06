package harnesscfg

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// HarnessManager keeps AGENTS.md and Skills in sync across CLI harnesses.
type HarnessManager struct {
	home     string
	trash    func(path string) error
	mutating bool
}

var harnesses = []struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Directory string `json:"directory"`
}{
	{ID: "claude", Name: "Claude", Directory: ".claude"},
	{ID: "agents", Name: "Agents", Directory: ".agents"},
	{ID: "droid", Name: "Droid", Directory: ".factory"},
	{ID: "codex", Name: "Codex", Directory: ".codex"},
}

// HarnessesCopy exposes the harness table to the API layer.
func HarnessesCopy() []struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Directory string `json:"directory"`
} {
	return append([]struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Directory string `json:"directory"`
	}{}, harnesses...)
}

func NewHarnessManager(home string, trash func(string) error) *HarnessManager {
	return &HarnessManager{home: home, trash: trash}
}

type harnessFolderRef struct {
	ID         string
	Name       string
	Directory  string
	Path       string
	SkillsPath string
}

func (h *HarnessManager) folder(id string) (harnessFolderRef, error) {
	for _, item := range harnesses {
		if item.ID == id {
			path := filepath.Join(h.home, item.Directory)
			return harnessFolderRef{ID: item.ID, Name: item.Name, Directory: item.Directory, Path: path, SkillsPath: filepath.Join(path, "skills")}, nil
		}
	}
	return harnessFolderRef{}, fmt.Errorf("Harness 无效")
}

type harnessSkill struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Linked    bool   `json:"linked"`
	Available bool   `json:"available"`
}

type harnessFolderInfo struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Path       string         `json:"path"`
	SkillsPath string         `json:"skillsPath"`
	Exists     bool           `json:"exists"`
	Skills     []harnessSkill `json:"skills"`
	Error      string         `json:"error,omitempty"`
}

type harnessInventoryResult struct {
	AgentsSource struct {
		Path   string `json:"path"`
		Exists bool   `json:"exists"`
	} `json:"agentsSource"`
	Harnesses []harnessFolderInfo `json:"harnesses"`
}

type harnessDocument struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type harnessOperationResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
}

func (h *HarnessManager) sourcePath() string {
	return filepath.Join(h.home, ".claude", "CLAUDE.md")
}

func (h *HarnessManager) skills(id string) ([]harnessSkill, error) {
	root, err := h.folder(id)
	if err != nil {
		return nil, err
	}
	skills := []harnessSkill{}
	var scan func(directory string, depth int) error
	scan = func(directory string, depth int) error {
		entries, err := os.ReadDir(directory)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".wide-") || entry.Name() == ".git" || entry.Name() == "node_modules" {
				continue
			}
			isDir := entry.IsDir()
			isLink := entry.Type()&os.ModeSymlink != 0
			if !isDir && !isLink {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			relativeID := toSlashPath(root.SkillsPath, path)
			info, statErr := os.Stat(path)
			if statErr != nil || !info.IsDir() {
				if isLink && statErr != nil {
					skills = append(skills, harnessSkill{ID: relativeID, Name: entry.Name(), Path: path, Linked: true, Available: false})
				}
				continue
			}
			manifest, manifestErr := os.Stat(filepath.Join(path, "SKILL.md"))
			if manifestErr == nil && !manifest.IsDir() {
				skills = append(skills, harnessSkill{ID: relativeID, Name: entry.Name(), Path: path, Linked: isLink, Available: true})
			} else if !isLink && depth < 3 {
				// Codex 的 .system 等分组目录也包含技能；不遍历链接分组，避免循环。
				if err := scan(path, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := scan(root.SkillsPath, 0); err != nil {
		return nil, err
	}
	sort.SliceStable(skills, func(i, j int) bool {
		a, b := skills[i], skills[j]
		if c := compareFoldNumeric(a.Name, b.Name); c != 0 {
			return c < 0
		}
		return a.ID < b.ID
	})
	return skills, nil
}

// compareFoldNumeric approximates localeCompare('en', {sensitivity:'base', numeric:true}).
func compareFoldNumeric(a, b string) int {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		ra, rb := rune(a[ai]), rune(b[bi])
		if unicode.IsDigit(ra) && unicode.IsDigit(rb) {
			na, da := readNumber(a, ai)
			nb, db := readNumber(b, bi)
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
			ai += da
			bi += db
			continue
		}
		la, lb := unicode.ToLower(ra), unicode.ToLower(rb)
		if la != lb {
			if la < lb {
				return -1
			}
			return 1
		}
		ai += len(string(ra))
		bi += len(string(rb))
	}
	switch {
	case len(a)-ai < len(b)-bi:
		return -1
	case len(a)-ai > len(b)-bi:
		return 1
	default:
		return 0
	}
}

func readNumber(s string, i int) (int, int) {
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	value := 0
	for _, ch := range s[i:j] {
		value = value*10 + int(ch-'0')
	}
	return value, j - i
}

func toSlashPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// Inventory returns AGENTS.md source info and folder metadata.
func (h *HarnessManager) Inventory(includeSkills bool) harnessInventoryResult {
	sourcePath := h.sourcePath()
	result := harnessInventoryResult{}
	result.AgentsSource.Path = sourcePath
	if info, err := os.Stat(sourcePath); err == nil && !info.IsDir() {
		result.AgentsSource.Exists = true
	}
	if includeSkills {
		result.Harnesses = h.SkillsInventory()
	}
	return result
}

// SkillsInventory lists skills for every harness folder.
func (h *HarnessManager) SkillsInventory() []harnessFolderInfo {
	var mu sync.Mutex
	var wg sync.WaitGroup
	out := make([]harnessFolderInfo, len(harnesses))
	for i, item := range harnesses {
		wg.Add(1)
		go func(index int, ref struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Directory string `json:"directory"`
		}) {
			defer wg.Done()
			path := filepath.Join(h.home, ref.Directory)
			entry := harnessFolderInfo{ID: ref.ID, Name: ref.Name, Path: path, SkillsPath: filepath.Join(path, "skills"), Skills: []harnessSkill{}}
			if _, err := os.Lstat(path); err == nil {
				entry.Exists = true
			} else if !os.IsNotExist(err) {
				entry.Error = err.Error()
			}
			skills, err := h.skills(ref.ID)
			if err != nil {
				entry.Error = err.Error()
			} else {
				entry.Skills = skills
			}
			mu.Lock()
			out[index] = entry
			mu.Unlock()
		}(i, item)
	}
	wg.Wait()
	return out
}

func (h *HarnessManager) readAgents() ([]byte, error) {
	path := h.sourcePath()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("未找到 %s", path)
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("CLAUDE.md 不是文件")
	}
	if info.Size() > 4*1024*1024 {
		return nil, fmt.Errorf("CLAUDE.md 超过 4 MB，无法读取")
	}
	return os.ReadFile(path)
}

// PreviewAgents reads CLAUDE.md for display.
func (h *HarnessManager) PreviewAgents() (harnessDocument, error) {
	content, err := h.readAgents()
	if err != nil {
		return harnessDocument{}, err
	}
	text := strings.TrimPrefix(string(content), "\uFEFF")
	return harnessDocument{Path: h.sourcePath(), Content: text}, nil
}

func (h *HarnessManager) contained(root string, target string) (string, error) {
	resolved, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("技能路径超出管理目录")
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("技能路径超出管理目录")
	}
	return resolved, nil
}

func (h *HarnessManager) exclusive(task func() (harnessOperationResult, error)) (harnessOperationResult, error) {
	if h.mutating {
		return harnessOperationResult{}, fmt.Errorf("Harness 操作正在执行，请稍后再试")
	}
	h.mutating = true
	defer func() { h.mutating = false }()
	return task()
}

func (h *HarnessManager) outcome(completed int, failures []string, message string) harnessOperationResult {
	result := harnessOperationResult{Success: len(failures) == 0, Completed: completed, Failed: len(failures)}
	if len(failures) > 0 {
		result.Message = fmt.Sprintf("%s；%d 项失败：%s", message, len(failures), strings.Join(failures, "；"))
	} else {
		result.Message = message + "。"
	}
	return result
}

func (h *HarnessManager) realDirectory(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s 是链接目录，请先改为普通目录再同步", path)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s 不是目录", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

// SyncAgents copies CLAUDE.md into every AGENTS.md location.
func (h *HarnessManager) SyncAgents() (result harnessOperationResult, err error) {
	return h.exclusive(func() (harnessOperationResult, error) {
		// 读取一次原始字节，保留源文件的编码和换行。
		content, err := h.readAgents()
		if err != nil {
			return harnessOperationResult{}, err
		}
		completed := 0
		var failures []string
		type targetRef struct{ name, path string }
		targets := []targetRef{}
		for _, id := range []string{"droid", "codex", "agents"} {
			folder, folderErr := h.folder(id)
			if folderErr != nil {
				return harnessOperationResult{}, folderErr
			}
			targets = append(targets, targetRef{folder.Name, folder.Path})
		}
		targets = append(targets, targetRef{"DSH", filepath.Join(h.home, ".dsh")})
		for _, folder := range targets {
			target := filepath.Join(folder.path, "AGENTS.md")
			stage := filepath.Join(folder.path, ".wide-"+randomHex()+".tmp")
			func() {
				defer func() {
					_ = os.Remove(stage)
				}()
				if err := h.realDirectory(folder.path); err != nil {
					failures = append(failures, fmt.Sprintf("%s：%s", folder.name, err.Error()))
					return
				}
				info, infoErr := os.Lstat(target)
				if infoErr == nil && info.Mode()&os.ModeSymlink == 0 && info.IsDir() {
					failures = append(failures, fmt.Sprintf("%s：AGENTS.md 不是文件", folder.name))
					return
				}
				if infoErr == nil && info.Mode()&os.ModeSymlink == 0 {
					if existing, readErr := os.ReadFile(target); readErr == nil && string(existing) == string(content) {
						completed++
						return
					}
				}
				if err := os.WriteFile(stage, content, 0o644); err != nil {
					failures = append(failures, fmt.Sprintf("%s：%s", folder.name, err.Error()))
					return
				}
				// 先准备新文件；已有文件进入回收站后，再放入新版本。
				if infoErr == nil {
					if err := h.trash(target); err != nil {
						failures = append(failures, fmt.Sprintf("%s：%s", folder.name, err.Error()))
						return
					}
				}
				if err := os.Rename(stage, target); err != nil {
					failures = append(failures, fmt.Sprintf("%s：%s", folder.name, err.Error()))
					return
				}
				completed++
			}()
		}
		return h.outcome(completed, failures, fmt.Sprintf("已同步 AGENTS.md 到 %d 个目录", completed)), nil
	})
}

// SyncSkills copies skills between Claude and Agents.
func (h *HarnessManager) SyncSkills(source any, skillID any) (harnessOperationResult, error) {
	sourceText, _ := source.(string)
	if sourceText != "claude" && sourceText != "agents" {
		return harnessOperationResult{}, fmt.Errorf("只能在 Claude 与 Agents 之间同步技能")
	}
	return h.exclusive(func() (harnessOperationResult, error) {
		targetID := "agents"
		if sourceText == "agents" {
			targetID = "claude"
		}
		sourceFolder, err := h.folder(sourceText)
		if err != nil {
			return harnessOperationResult{}, err
		}
		targetFolder, err := h.folder(targetID)
		if err != nil {
			return harnessOperationResult{}, err
		}
		selected, err := h.selectedSkills(sourceText, skillID)
		if err != nil {
			return harnessOperationResult{}, err
		}
		if len(selected) == 0 {
			return h.outcome(0, nil, "没有可同步的技能"), nil
		}
		if err := h.realDirectory(targetFolder.Path); err != nil {
			return harnessOperationResult{}, err
		}
		if err := h.realDirectory(targetFolder.SkillsPath); err != nil {
			return harnessOperationResult{}, err
		}
		sourceRoot, err := filepath.EvalSymlinks(sourceFolder.SkillsPath)
		if err != nil {
			return harnessOperationResult{}, err
		}
		targetRoot, err := filepath.EvalSymlinks(targetFolder.SkillsPath)
		if err != nil {
			return harnessOperationResult{}, err
		}
		normalize := func(path string) string {
			if isWindowsRuntime() {
				return strings.ToLower(path)
			}
			return path
		}
		if normalize(sourceRoot) == normalize(targetRoot) {
			return harnessOperationResult{}, fmt.Errorf("源 skills 与目标 skills 指向同一个目录，无法复制")
		}
		completed := 0
		var failures []string
		for _, skill := range selected {
			target, err := h.contained(targetFolder.SkillsPath, filepath.Join(targetFolder.SkillsPath, filepath.FromSlash(skill.ID)))
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s：%s", skill.Name, err.Error()))
				continue
			}
			stage, err := h.contained(targetFolder.SkillsPath, filepath.Join(targetFolder.SkillsPath, ".wide-"+randomHex()))
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s：%s", skill.Name, err.Error()))
				continue
			}
			func() {
				defer func() {
					_ = os.RemoveAll(stage)
				}()
				if !skill.Available {
					failures = append(failures, fmt.Sprintf("%s：源技能链接已失效", skill.Name))
					return
				}
				physicalSource, err := filepath.EvalSymlinks(skill.Path)
				if err != nil {
					failures = append(failures, fmt.Sprintf("%s：%s", skill.Name, err.Error()))
					return
				}
				destinationRelative, err := filepath.Rel(normalize(physicalSource), normalize(targetRoot))
				if err != nil || (destinationRelative != ".." && !strings.HasPrefix(destinationRelative, ".."+string(filepath.Separator)) && !filepath.IsAbs(destinationRelative)) {
					failures = append(failures, fmt.Sprintf("%s：目标 skills 位于源技能内，无法复制", skill.Name))
					return
				}
				// 中间分组必须是实际目录；目标技能本身若为链接，替换其链接而不写入链接目标。
				parent := targetFolder.SkillsPath
				rel, _ := filepath.Rel(targetFolder.SkillsPath, filepath.Dir(target))
				for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
					if part == "" || part == "." {
						continue
					}
					parent = filepath.Join(parent, part)
					if err := h.realDirectory(parent); err != nil {
						failures = append(failures, fmt.Sprintf("%s：%s", skill.Name, err.Error()))
						return
					}
				}
				if err := copyTree(skill.Path, stage); err != nil {
					failures = append(failures, fmt.Sprintf("%s：%s", skill.Name, err.Error()))
					return
				}
				if _, err := os.Lstat(target); err == nil {
					if err := h.trash(target); err != nil {
						failures = append(failures, fmt.Sprintf("%s：%s", skill.Name, err.Error()))
						return
					}
				}
				if err := os.Rename(stage, target); err != nil {
					failures = append(failures, fmt.Sprintf("%s：%s", skill.Name, err.Error()))
					return
				}
				completed++
			}()
		}
		return h.outcome(completed, failures, fmt.Sprintf("已同步 %d 个技能到 %s", completed, targetFolder.Name)), nil
	})
}

func (h *HarnessManager) selectedSkills(id string, skillID any) ([]harnessSkill, error) {
	skills, err := h.skills(id)
	if err != nil {
		return nil, err
	}
	if skillID == nil {
		return skills, nil
	}
	text, ok := skillID.(string)
	if !ok {
		return nil, fmt.Errorf("技能标识无效")
	}
	for _, skill := range skills {
		if skill.ID == text {
			folder, _ := h.folder(id)
			if _, err := h.contained(folder.SkillsPath, skill.Path); err != nil {
				return nil, err
			}
			return []harnessSkill{skill}, nil
		}
	}
	return nil, fmt.Errorf("技能已不存在，请刷新列表")
}

// DeleteSkills moves a skill to the recycle bin.
func (h *HarnessManager) DeleteSkills(id any, skillID any) (harnessOperationResult, error) {
	idText, _ := id.(string)
	text, ok := skillID.(string)
	if !ok || text == "" {
		return harnessOperationResult{}, fmt.Errorf("请选择要删除的技能")
	}
	return h.exclusive(func() (harnessOperationResult, error) {
		folder, err := h.folder(idText)
		if err != nil {
			return harnessOperationResult{}, err
		}
		selected, err := h.selectedSkills(folder.ID, skillID)
		if err != nil {
			return harnessOperationResult{}, err
		}
		completed := 0
		var failures []string
		for _, skill := range selected {
			path, err := h.contained(folder.SkillsPath, skill.Path)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s：%s", skill.Name, err.Error()))
				continue
			}
			// 不解析符号链接，删除链接只将链接本身移到回收站。
			if err := h.trash(path); err != nil {
				failures = append(failures, fmt.Sprintf("%s：%s", skill.Name, err.Error()))
				continue
			}
			completed++
		}
		return h.outcome(completed, failures, fmt.Sprintf("已将 %d 个技能移到回收站", completed)), nil
	})
}

func randomHex() string {
	buffer := make([]byte, 16)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}

// copyTree copies a directory tree, dereferencing symlinks.
func copyTree(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	}
	if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyTree(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

var _ = io.Discard
