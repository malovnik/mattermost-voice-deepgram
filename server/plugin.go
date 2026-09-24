package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const pluginID = "com.malovnik.voice-deepgram"
const sourceProp = "voice_source_post_id"

type Plugin struct {
	plugin.MattermostPlugin
	configMu      sync.RWMutex
	configuration configuration
	botID         string
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	wake          chan struct{}
	client        *http.Client
}

func (p *Plugin) OnActivate() error {
	if err := p.OnConfigurationChange(); err != nil {
		return err
	}
	id, err := p.API.EnsureBotUser(&model.Bot{Username: "voice-transcriber", DisplayName: "Расшифровка аудио", Description: "Voice Messages · Deepgram"})
	if err != nil {
		return fmt.Errorf("create transcription bot: %w", err)
	}
	bot, botErr := p.API.GetBot(id, false)
	if botErr != nil || bot == nil || bot.OwnerId != pluginID {
		return errors.New("voice-transcriber must be a bot owned by this plugin; resolve the username conflict before activation")
	}
	p.botID = id
	if err := p.API.RegisterCommand(&model.Command{Trigger: "voice", AutoComplete: true, AutoCompleteDesc: "Расшифровать аудио: /voice transcribe POST_ID", AutoCompleteHint: "[transcribe POST_ID]"}); err != nil {
		return err
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.wake = make(chan struct{}, 1)
	p.client = &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	p.wg.Add(1)
	go func() { defer p.wg.Done(); p.run() }()
	return nil
}
func (p *Plugin) OnDeactivate() error {
	if p.cancel != nil {
		p.cancel()
		p.wg.Wait()
	}
	return nil
}

func audioMIME(f *model.FileInfo) string {
	switch strings.ToLower(filepath.Ext(f.Name)) {
	case ".webm":
		return "audio/webm"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".m4a", ".mp4":
		if strings.HasPrefix(f.MimeType, "video/") && !strings.HasPrefix(f.Name, "voice-message-") {
			return ""
		}
		return "audio/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".aac":
		return "audio/aac"
	}
	return ""
}

func (p *Plugin) firstAudio(post *model.Post, c configuration, manual bool) (*model.FileInfo, error) {
	return p.findAudio(post, c, manual, false)
}

func (p *Plugin) findAudio(post *model.Post, c configuration, manual, allowUnattached bool) (*model.FileInfo, error) {
	for _, id := range post.FileIds {
		f, err := p.API.GetFileInfo(id)
		if err != nil {
			return nil, errors.New("Не удалось прочитать вложение.")
		}
		if (f.PostId != post.Id && !(allowUnattached && f.PostId == "")) || f.DeleteAt != 0 {
			continue
		}
		if audioMIME(f) == "" {
			continue
		}
		if !manual && !c.TranscribeAllAudio && !strings.HasPrefix(f.Name, "voice-message-") {
			continue
		}
		if f.Size <= 0 || f.Size > c.maxBytes() {
			return nil, fmt.Errorf("Аудиофайл превышает лимит %s МиБ или пуст.", c.MaxFileSizeMB)
		}
		return f, nil
	}
	return nil, errors.New("В сообщении нет поддерживаемого аудио: WebM, Ogg, Opus, M4A, MP3, WAV, FLAC, AAC.")
}

func (p *Plugin) canRead(user, channel string) bool {
	if _, err := p.API.GetChannelMember(channel, user); err != nil {
		return false
	}
	return p.API.HasPermissionToChannel(user, channel, model.PermissionReadChannel)
}

func (p *Plugin) MessageHasBeenPosted(_ *plugin.Context, post *model.Post) {
	if post.UserId == p.botID || post.DeleteAt != 0 || post.Type != "" || len(post.FileIds) == 0 {
		return
	}
	c := p.config()
	if c.DeepgramAPIKey == "" {
		return
	}
	if _, err := p.findAudio(post, c, false, true); err != nil {
		return
	}
	if err := p.enqueue(post.Id); err != nil {
		p.API.LogWarn("Voice queue unavailable", "post_id", post.Id)
	}
}

func (p *Plugin) requestTranscription(user, id string) error {
	if !model.IsValidId(id) {
		return errors.New("Нужен ID сообщения (26 символов, последний фрагмент постоянной ссылки).")
	}
	post, err := p.API.GetPost(id)
	if err != nil || post.DeleteAt != 0 || !p.canRead(user, post.ChannelId) {
		return errors.New("Сообщение недоступно.")
	}
	if post.UserId != user && !p.API.HasPermissionTo(user, model.PermissionManageSystem) {
		return errors.New("Повторную расшифровку запускает автор сообщения или администратор.")
	}
	if p.config().DeepgramAPIKey == "" {
		return errors.New("Администратор ещё не указал ключ Deepgram. Голосовые сообщения уже можно отправлять.")
	}
	if _, e := p.firstAudio(post, p.config(), true); e != nil {
		return e
	}
	return p.enqueue(id)
}

func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	message := "Запись: скрепка → Голосовое сообщение. После прослушивания добавьте запись к сообщению и отправьте его. Для расшифровки своего опубликованного аудио: `/voice transcribe POST_ID`. На телефоне прикрепите файл из диктофона."
	fields := strings.Fields(args.Command)
	if len(fields) == 3 && fields[1] == "transcribe" {
		if e := p.requestTranscription(args.UserId, fields[2]); e != nil {
			message = e.Error()
		} else {
			message = "Запрос принят. Готовая расшифровка появится в ветке; уже готовый результат повторно не обрабатывается."
		}
	}
	return &model.CommandResponse{ResponseType: model.CommandResponseTypeEphemeral, Text: message}, nil
}

func (p *Plugin) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	user := r.Header.Get("Mattermost-User-ID")
	if !model.IsValidId(user) {
		http.Error(w, `{"error":"Требуется вход в Mattermost."}`, 401)
		return
	}
	if r.URL.Path == "/config" && r.Method == http.MethodGet {
		c := p.config()
		_ = json.NewEncoder(w).Encode(map[string]any{"configured": c.DeepgramAPIKey != "", "maxRecordingSeconds": c.seconds(), "maxFileBytes": c.maxBytes()})
		return
	}
	if r.URL.Path == "/transcribe" && r.Method == http.MethodPost {
		// No CORS. This non-simple header also blocks cross-origin form CSRF.
		if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
			http.Error(w, `{"error":"Недопустимый запрос."}`, 403)
			return
		}
		var input struct {
			PostID string `json:"post_id"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, `{"error":"Неверный JSON."}`, 400)
			return
		}
		if err := p.requestTranscription(user, input.PostID); err != nil {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	}
	http.NotFound(w, r)
}
