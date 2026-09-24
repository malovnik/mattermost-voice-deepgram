package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

const jobPrefix = "job_"

type job struct {
	PostID   string `json:"post_id"`
	Attempts int    `json:"attempts"`
	Next     int64  `json:"next"`
	Lease    string `json:"lease"`
}

func encodeJob(j job) []byte { b, _ := json.Marshal(j); return b }

func (p *Plugin) enqueue(id string) error {
	// A CAS insertion deduplicates concurrent hooks, retries and multiple server nodes.
	_, err := p.API.KVCompareAndSet(jobPrefix+id, nil, encodeJob(job{PostID: id}))
	if err != nil {
		return errors.New("Очередь недоступна. Повторите команду позже.")
	}
	if p.wake != nil {
		select {
		case p.wake <- struct{}{}:
		default:
		}
	}
	return nil
}

func (p *Plugin) run() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		p.scan()
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
		case <-p.wake:
		}
	}
}
func (p *Plugin) scan() {
	if p.config().DeepgramAPIKey == "" {
		return
	}
	// Snapshot keys before deleting to avoid skipping jobs as pages shift.
	var keys []string
	for page := 0; ; page++ {
		if p.ctx.Err() != nil {
			return
		}
		batch, err := p.API.KVList(page, 200)
		if err != nil {
			p.API.LogWarn("Voice queue scan failed")
			return
		}
		for _, key := range batch {
			if strings.HasPrefix(key, jobPrefix) {
				keys = append(keys, key)
			}
		}
		if len(batch) < 200 {
			break
		}
	}
	for _, key := range keys {
		if p.ctx.Err() != nil {
			return
		}
		p.process(key)
	}
}

func (p *Plugin) process(key string) {
	old, e := p.API.KVGet(key)
	if e != nil || len(old) == 0 {
		return
	}
	var j job
	if json.Unmarshal(old, &j) != nil || !model.IsValidId(j.PostID) {
		return
	}
	if j.Next > time.Now().Unix() {
		return
	}
	j.Next = time.Now().Add(3 * time.Minute).Unix()
	j.Lease = model.NewId()
	j.Attempts++
	held := encodeJob(j)
	ok, e := p.API.KVCompareAndSet(key, old, held)
	if e != nil || !ok {
		return
	}
	err := p.perform(j, key, held)
	if p.ctx.Err() != nil {
		// Graceful restart preserves work and does not consume a provider retry.
		j.Next = 0
		j.Lease = ""
		j.Attempts--
		_, _ = p.API.KVCompareAndSet(key, held, encodeJob(j))
		return
	}
	var te *transcriptionError
	if errors.As(err, &te) && te.retryable && j.Attempts < 3 {
		j.Next = time.Now().Add(time.Duration(j.Attempts*j.Attempts) * 15 * time.Second).Unix()
		j.Lease = ""
		_, _ = p.API.KVCompareAndSet(key, held, encodeJob(j))
		return
	}
	if err != nil {
		// A failed notification leaves the job recoverable; do not silently drop a task.
		if e := p.publishResult(j.PostID, "", err, key, held); e != nil {
			j.Next = time.Now().Add(5 * time.Minute).Unix()
			j.Lease = ""
			_, _ = p.API.KVCompareAndSet(key, held, encodeJob(j))
			return
		}
	}
	_, _ = p.API.KVCompareAndDelete(key, held)
}

func (p *Plugin) owns(key string, held []byte) bool {
	b, e := p.API.KVGet(key)
	var j job
	return e == nil && bytes.Equal(b, held) && json.Unmarshal(b, &j) == nil && j.Next > time.Now().Unix() && p.ctx.Err() == nil
}

func (p *Plugin) usablePost(id string) (*model.Post, error) {
	post, e := p.API.GetPost(id)
	if e != nil {
		if e.StatusCode == 404 {
			return nil, nil
		}
		return nil, dgError("mattermost_unavailable", true)
	}
	if post.DeleteAt != 0 || !p.canRead(post.UserId, post.ChannelId) {
		return nil, nil
	}
	channel, e := p.API.GetChannel(post.ChannelId)
	if e != nil {
		return nil, dgError("mattermost_unavailable", true)
	}
	if channel.DeleteAt != 0 {
		return nil, nil
	}
	return post, nil
}

func (p *Plugin) existingReply(post *model.Post) (*model.Post, error) {
	thread, e := p.API.GetPostThread(post.Id)
	if e != nil {
		return nil, dgError("mattermost_unavailable", true)
	}
	for _, candidate := range thread.Posts {
		if candidate.DeleteAt == 0 && candidate.UserId == p.botID && candidate.GetProp(sourceProp) == post.Id {
			return candidate, nil
		}
	}
	return nil, nil
}

func (p *Plugin) perform(j job, key string, held []byte) error {
	post, err := p.usablePost(j.PostID)
	if err != nil || post == nil {
		return err
	}
	previous, err := p.existingReply(post)
	if err != nil {
		return err
	}
	if previous != nil && previous.GetProp("voice_status") == "done" {
		return nil
	}
	c := p.config()
	if c.DeepgramAPIKey == "" {
		return dgError("not_configured", true)
	}
	f, err := p.firstAudio(post, c, true)
	if err != nil {
		return dgError("audio_unavailable", false)
	}
	data, e := p.API.GetFile(f.Id)
	if e != nil {
		return dgError("file_unavailable", true)
	}
	if len(data) == 0 || int64(len(data)) > c.maxBytes() {
		return dgError("file_size", false)
	}
	ctx, cancel := context.WithTimeout(p.ctx, 90*time.Second)
	defer cancel()
	transcript, err := transcribe(ctx, p.client, c, data, audioMIME(f))
	if err != nil {
		return err
	}
	return p.publishResult(post.Id, transcript, nil, key, held)
}

func (p *Plugin) publishResult(id, text string, cause error, key string, held []byte) error {
	if !p.owns(key, held) {
		return dgError("lease_lost", true)
	}
	post, err := p.usablePost(id)
	if err != nil || post == nil {
		return err
	}
	previous, err := p.existingReply(post)
	if err != nil {
		return err
	}
	if previous != nil && previous.GetProp("voice_status") == "done" {
		return nil
	}
	status := "done"
	body := "**Расшифровка аудио**\n\n" + safeTranscript(text)
	if text == "" {
		body = "**Расшифровка аудио**\n\nРечь не обнаружена. Запись доступна в исходном сообщении."
	}
	if cause != nil {
		status = "failed"
		code := "processing_error"
		var te *transcriptionError
		if errors.As(cause, &te) {
			code = te.code
		}
		body = fmt.Sprintf("Не удалось расшифровать аудио (`%s`). Запись сохранена. Автор может повторить: `/voice transcribe %s`.", code, id)
	}
	base := ""
	if c := p.API.GetConfig(); c.ServiceSettings.SiteURL != nil {
		if site, e := url.Parse(*c.ServiceSettings.SiteURL); e == nil {
			base = strings.TrimRight(site.Path, "/")
		}
	}
	body += "\n\n[Исходное аудио](" + base + "/_redirect/pl/" + id + ")"
	if previous != nil {
		previous.Message = body
		previous.AddProp("voice_status", status)
		if _, e := p.API.UpdatePost(previous); e != nil {
			return dgError("mattermost_unavailable", true)
		}
		return nil
	}
	root := post.RootId
	if root == "" {
		root = post.Id
	}
	reply := &model.Post{UserId: p.botID, ChannelId: post.ChannelId, RootId: root, Message: body}
	reply.AddProp(sourceProp, id)
	reply.AddProp("voice_status", status)
	if _, e := p.API.CreatePost(reply); e != nil {
		return dgError("mattermost_unavailable", true)
	}
	return nil
}
