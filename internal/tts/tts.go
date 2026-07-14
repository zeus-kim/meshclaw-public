package tts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Options struct {
	Text      string `json:"text"`
	Engine    string `json:"engine,omitempty"`
	Voice     string `json:"voice,omitempty"`
	OutputDir string `json:"output_dir,omitempty"`
	Basename  string `json:"basename,omitempty"`
}

type Result struct {
	Kind      string    `json:"kind"`
	Engine    string    `json:"engine"`
	Voice     string    `json:"voice,omitempty"`
	Path      string    `json:"path"`
	TempPath  string    `json:"temp_path,omitempty"`
	TextPath  string    `json:"text_path"`
	Command   []string  `json:"command"`
	Convert   []string  `json:"convert,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func Synthesize(opts Options) (Result, error) {
	text := strings.TrimSpace(opts.Text)
	if text == "" {
		return Result{}, fmt.Errorf("tts text is required")
	}
	text = NormalizePronunciation(text, opts)
	outputDir := strings.TrimSpace(opts.OutputDir)
	if outputDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Result{}, err
		}
		outputDir = filepath.Join(home, ".meshclaw", "audio")
	}
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return Result{}, err
	}
	base := sanitizeBase(opts.Basename)
	if base == "" {
		base = "meshclaw-brief-" + time.Now().UTC().Format("20060102T150405Z")
	}
	textPath := filepath.Join(outputDir, base+".txt")
	if err := os.WriteFile(textPath, []byte(text+"\n"), 0600); err != nil {
		return Result{}, err
	}
	engine := strings.ToLower(strings.TrimSpace(opts.Engine))
	if engine == "" {
		engine = defaultEngine()
	}
	if engine == "edge" || engine == "edge-tts" {
		return synthesizeEdge(opts, textPath, outputDir, base)
	}
	return synthesizeMacOSSay(opts, textPath, outputDir, base)
}

var argosTokenPattern = regexp.MustCompile(`(?i)\bargos\b`)

func NormalizePronunciation(text string, opts Options) string {
	lang := ttsLanguage(opts)
	switch lang {
	case "ko":
		return argosTokenPattern.ReplaceAllString(text, "아르고스")
	case "ja":
		return argosTokenPattern.ReplaceAllString(text, "アルゴス")
	case "zh":
		return argosTokenPattern.ReplaceAllString(text, "阿尔戈斯")
	default:
		return text
	}
}

func ttsLanguage(opts Options) string {
	voice := strings.ToLower(strings.TrimSpace(opts.Voice))
	explicitVoice := voice != ""
	if voice == "" {
		voice = strings.ToLower(strings.TrimSpace(firstNonEmpty(os.Getenv("MESHCLAW_TTS_EDGE_VOICE"), os.Getenv("MESHCLAW_TTS_VOICE"))))
	}
	switch {
	case strings.HasPrefix(voice, "ko-") || strings.Contains(voice, "korean") || voice == "yuna":
		return "ko"
	case strings.HasPrefix(voice, "ja-") || strings.Contains(voice, "japanese") || voice == "kyoko" || voice == "otoya":
		return "ja"
	case strings.HasPrefix(voice, "zh-") || strings.Contains(voice, "chinese") || voice == "tingting" || voice == "mei-jia" || voice == "sin-ji":
		return "zh"
	}
	if explicitVoice || voice != "" {
		return ""
	}
	engine := strings.ToLower(strings.TrimSpace(opts.Engine))
	if engine == "" || engine == "edge" || engine == "edge-tts" {
		return "ko"
	}
	return ""
}

func defaultEngine() string {
	return strings.ToLower(strings.TrimSpace(firstNonEmpty(os.Getenv("MESHCLAW_TTS_ENGINE"), "edge-tts")))
}

func synthesizeEdge(opts Options, textPath, outputDir, base string) (Result, error) {
	audioPath := filepath.Join(outputDir, base+".mp3")
	voice := strings.TrimSpace(opts.Voice)
	if voice == "" {
		voice = firstNonEmpty(os.Getenv("MESHCLAW_TTS_EDGE_VOICE"), os.Getenv("MESHCLAW_TTS_VOICE"), "ko-KR-SunHiNeural")
	}
	binary := firstNonEmpty(os.Getenv("MESHCLAW_EDGE_TTS"), "edge-tts")
	cmdArgs := []string{"--voice", voice, "--file", textPath, "--write-media", audioPath}
	command := append([]string{binary}, cmdArgs...)
	if out, err := exec.Command(binary, cmdArgs...).CombinedOutput(); err != nil {
		fallback := append([]string{"-m", "edge_tts"}, cmdArgs...)
		fallbackOut, fallbackErr := exec.Command("python3", fallback...).CombinedOutput()
		if fallbackErr != nil {
			return Result{}, fmt.Errorf("edge-tts failed: %v %s; python3 -m edge_tts failed: %v %s", err, strings.TrimSpace(string(out)), fallbackErr, strings.TrimSpace(string(fallbackOut)))
		}
		command = append([]string{"python3"}, fallback...)
	}
	return Result{
		Kind:      "meshclaw_tts_result",
		Engine:    "edge-tts",
		Voice:     voice,
		Path:      audioPath,
		TextPath:  textPath,
		Command:   command,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func synthesizeMacOSSay(opts Options, textPath, outputDir, base string) (Result, error) {
	tempPath := filepath.Join(outputDir, base+".aiff")
	audioPath := filepath.Join(outputDir, base+".m4a")
	voice := strings.TrimSpace(opts.Voice)
	if voice == "" {
		voice = firstNonEmpty(os.Getenv("MESHCLAW_TTS_VOICE"), "Yuna")
	}
	cmdArgs := []string{"-v", voice, "-o", tempPath, "-f", textPath}
	cmd := exec.Command("say", cmdArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		cmdArgs = []string{"-o", tempPath, "-f", textPath}
		cmd = exec.Command("say", cmdArgs...)
		if fallbackOut, fallbackErr := cmd.CombinedOutput(); fallbackErr != nil {
			return Result{}, fmt.Errorf("say failed: %s; fallback failed: %s", strings.TrimSpace(string(out)), strings.TrimSpace(string(fallbackOut)))
		}
		voice = ""
	}
	convertArgs := []string{"-f", "m4af", "-d", "aac", tempPath, audioPath}
	if out, err := exec.Command("afconvert", convertArgs...).CombinedOutput(); err != nil {
		audioPath = tempPath
		convertArgs = nil
		_ = out
	} else {
		_ = os.Remove(tempPath)
		tempPath = ""
	}
	return Result{
		Kind:      "meshclaw_tts_result",
		Engine:    "macos-say",
		Voice:     voice,
		Path:      audioPath,
		TempPath:  tempPath,
		TextPath:  textPath,
		Command:   append([]string{"say"}, cmdArgs...),
		Convert:   append([]string{"afconvert"}, convertArgs...),
		CreatedAt: time.Now().UTC(),
	}, nil
}

func sanitizeBase(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
