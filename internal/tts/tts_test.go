package tts

import "testing"

func TestDefaultEngineIsEdgeTTS(t *testing.T) {
	t.Setenv("MESHCLAW_TTS_ENGINE", "")
	if got := defaultEngine(); got != "edge-tts" {
		t.Fatalf("default engine = %q, want edge-tts", got)
	}
}

func TestDefaultEngineHonorsEnvironment(t *testing.T) {
	t.Setenv("MESHCLAW_TTS_ENGINE", "local")
	if got := defaultEngine(); got != "local" {
		t.Fatalf("default engine = %q, want local", got)
	}
}

func TestNormalizePronunciationArgosByLanguage(t *testing.T) {
	cases := []struct {
		name  string
		voice string
		want  string
	}{
		{name: "korean", voice: "ko-KR-SunHiNeural", want: "아르고스 운영 보안 브리핑입니다."},
		{name: "japanese", voice: "ja-JP-NanamiNeural", want: "アルゴス 운영 보안 브리핑입니다."},
		{name: "chinese", voice: "zh-CN-XiaoxiaoNeural", want: "阿尔戈斯 운영 보안 브리핑입니다."},
		{name: "english", voice: "en-US-AriaNeural", want: "Argos 운영 보안 브리핑입니다."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizePronunciation("Argos 운영 보안 브리핑입니다.", Options{Voice: tc.voice})
			if got != tc.want {
				t.Fatalf("normalized=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizePronunciationDefaultEdgeIsKorean(t *testing.T) {
	t.Setenv("MESHCLAW_TTS_EDGE_VOICE", "")
	t.Setenv("MESHCLAW_TTS_VOICE", "")
	got := NormalizePronunciation("Argos DevOps 보안 음성 보고입니다.", Options{Engine: "edge-tts"})
	if got != "아르고스 DevOps 보안 음성 보고입니다." {
		t.Fatalf("normalized=%q", got)
	}
}
